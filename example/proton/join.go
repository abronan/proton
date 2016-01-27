package main

import (
	"log"
	"net"
	"os"

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

	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalf("can't get hostname")
	}

	id := proton.GenID(hostname)
	node := proton.NewNode(id)

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

	info := &proton.NodeInfo{
		ID:   id,
		Addr: hosts[0],
	}

	resp, err := p.Join(context.Background(), info)
	if err != nil {
		log.Fatalf("could not join: %v", err)
	}
	log.Printf("Ack: %s", resp)

	go server.Serve(lis)

	// TODO loop and print values

	select {}
}
