package main

import (
	"net"
	"os"

	"google.golang.org/grpc"

	log "github.com/Sirupsen/logrus"
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
	s := grpc.NewServer()

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("can't get hostname")
	}

	node := proton.NewNode(hostname, []raft.Peer{})
	node.Raft.Campaign(node.Ctx)
	go node.Start()

	log.Println("Starting raft transport layer..")
	proton.Register(s, nil)

	go s.Serve(lis)

	select {}
}
