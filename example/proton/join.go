package main

import (
	"fmt"
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
	node := proton.NewNode(id, hosts[0])

	lis, err := net.Listen("tcp", hosts[0])
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	server := grpc.NewServer()
	proton.Register(server, node)

	conn, err := grpc.Dial(joinAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	p := proton.NewProtonClient(conn)

	// Start raft
	go node.Start()
	go server.Serve(lis)

	info := &proton.NodeInfo{
		ID:   id,
		Addr: hosts[0],
	}

	resp, err := p.JoinCluster(context.Background(), info)
	if err != nil {
		log.Fatalf("could not join: %v", err)
	}

	for _, info := range resp.GetInfo() {
		err := node.RegisterNode(info)
		if err != nil {
			log.Fatal(err)
		}
	}

	_, err = p.JoinRaft(context.Background(), info)
	if err != nil {
		log.Fatalf("could not join: %v", err)
	}
	if err != nil {
		log.Fatal(err)
	}

	ticker := time.NewTicker(time.Second * 20)
	go func() {
		for _ = range ticker.C {
			for k, v := range node.PStore {
				fmt.Printf("%v = %v\n", k, v)
			}
		}
	}()

	select {}
}
