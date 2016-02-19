package main

import (
	"log"
	"net"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
)

func join(c *cli.Context) {
	var (
		err error
	)

	hosts := c.StringSlice("host")
	if c.IsSet("host") || c.IsSet("H") {
		hosts = hosts[1:]
	}

	lis, err := net.Listen("tcp", hosts[0])
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	server := grpc.NewServer()

	joinAddr := c.String("join")
	hostname := c.String("hostname")

	id := proton.GenID(hostname)
	node := proton.NewRaftNode(id, hosts[0], 1, server, lis, c.Bool("withRaftLogs"), nil, handler)

	proton.Register(server, node)

	client, err := proton.GetRaftClient(joinAddr, 2*time.Second)
	if err != nil {
		log.Fatal("couldn't initialize client connection")
	}

	// Start raft
	go node.Start()
	go server.Serve(lis)

	info := &proton.NodeInfo{
		ID:   id,
		Addr: hosts[0],
	}

	resp, err := client.JoinRaft(context.Background(), info)
	if err != nil {
		log.Fatalf("could not join: %v", err)
	}

	err = node.RegisterNodes(resp.GetNodes())
	if err != nil {
		log.Fatal(err)
	}

	select {}
}
