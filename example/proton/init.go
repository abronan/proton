package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"strconv"
	"time"

	"google.golang.org/grpc"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
	"github.com/coreos/etcd/raft"
)

func initcluster(c *cli.Context) {
	hosts := c.StringSlice("host")
	if c.IsSet("host") || c.IsSet("H") {
		hosts = hosts[1:]
	}

	lis, err := net.Listen("tcp", hosts[0])
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	server := grpc.NewServer()

	hostname := c.String("hostname")

	id := proton.GenID(hostname)

	if c.Bool("withRaftLogs") {
		raftLogger = &raft.DefaultLogger{Logger: log.New(ioutil.Discard, "", 0)}
	}

	cfg := proton.DefaultNodeConfig()
	cfg.Logger = raftLogger

	node, err := proton.NewNode(id, hosts[0], cfg, handler)
	if err != nil {
		log.Fatal("Can't initialize raft node")
	}

	node.Campaign(node.Ctx)
	go node.Start()

	log.Println("Starting raft transport layer..")
	proton.Register(server, node)

	go server.Serve(lis)

	ticker := time.NewTicker(time.Second * 10)
	go func() {
		i := 0
		for _ = range ticker.C {
			s := strconv.Itoa(i)
			pair, err := proton.EncodePair("key"+s, []byte("myvalue"+s))
			if err != nil {
				log.Fatal("Can't encode KV pair")
			}

			if node.IsLeader() {
				fmt.Println("---> Leading the raft")
			}

			node.Propose(node.Ctx, pair)
			i++
		}
	}()

	select {}
}
