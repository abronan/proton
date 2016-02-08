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

	joinAddr := c.String("join")
	hostname := c.String("hostname")

	id := proton.GenID(hostname)
	node := proton.NewNode(id, hosts[0], c.Bool("withRaftLogs"), nil, handler)

	lis, err := net.Listen("tcp", hosts[0])
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	server := grpc.NewServer()
	proton.Register(server, node)

	client, err := proton.GetProtonClient(joinAddr, 2*time.Second)
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

	resp, err := client.Client.JoinCluster(context.Background(), info)
	if err != nil {
		log.Fatalf("could not join: %v", err)
	}

	for _, info := range resp.GetInfo() {
		err := node.RegisterNode(info)
		if err != nil {
			log.Fatal(err)
		}
	}

	_, err = client.Client.JoinRaft(context.Background(), info)
	if err != nil {
		log.Fatal(err)
	}

	select {}
}
