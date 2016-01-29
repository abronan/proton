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
	s := grpc.NewServer()

	hostname := c.String("hostname")

	id := proton.GenID(hostname)

	node := proton.NewNode(id, hosts[0])
	node.Raft.Campaign(node.Ctx)
	go node.Start()

	log.Println("Starting raft transport layer..")
	proton.Register(s, node)

	go s.Serve(lis)

	ticker := time.NewTicker(time.Second * 10)
	go func() {
		i := 0
		for _ = range ticker.C {
			s := strconv.Itoa(i)
			pair, err := proton.EncodePair("key"+s, []byte("myvalue"+s))
			if err != nil {
				log.Fatal("Can't encode KV pair")
			}

			node.Raft.Propose(node.Ctx, pair)
			i++
			for k, v := range node.PStore {
				fmt.Printf("%v = %v\n", k, v)
			}
		}
	}()

	select {}
}
