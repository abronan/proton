package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
	"github.com/coreos/etcd/raft"
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

	if c.Bool("withRaftLogs") {
		raftLogger = &raft.DefaultLogger{Logger: log.New(ioutil.Discard, "", 0)}
	}

	id := proton.GenID(hostname)
	cfg := proton.DefaultNodeConfig()
	cfg.Logger = raftLogger

	node, err := proton.NewNode(id, hosts[0], cfg, handler)
	if err != nil {
		log.Fatal("Can't initialize raft node")
	}

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

	ticker := time.NewTicker(time.Second * 5)
	go func() {
		for _ = range ticker.C {
			fmt.Println("----- Leader is: ", node.Leader())
		}
	}()

	select {}
}
