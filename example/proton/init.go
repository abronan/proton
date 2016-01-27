package main

import (
	"net"
	"os"
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

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("can't get hostname")
	}

	id := proton.GenID(hostname)

	node := proton.NewNode(id)
	node.Raft.Campaign(node.Ctx)
	go node.Start()

	log.Println("Starting raft transport layer..")
	proton.Register(s, node)

	go s.Serve(lis)

	ticker := time.NewTicker(time.Second * 20)
	go func() {
		for _ = range ticker.C {
			node.Raft.Propose(node.Ctx, []byte("mykey1:myvalue1"))
			// TODO print values
		}
	}()

	select {}
}
