package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
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

	node := proton.NewRaftNode(id, hosts[0], 1, server, lis, c.Bool("withRaftLogs"), nil, handler)
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
