package main

import (
	"fmt"
	"log"
	"time"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
	"golang.org/x/net/context"
)

func merge(c *cli.Context) {
	hosts := c.StringSlice("host")
	if c.IsSet("host") || c.IsSet("H") {
		hosts = hosts[1:]
	}

	joinAddr := c.String("join")

	client, err := proton.GetRaftClient(hosts[0], 2*time.Second)
	if err != nil {
		log.Fatal("couldn't initialize client connection")
	}

	_, err = client.Merge(context.Background(),
		&proton.MergeRequest{
			&proton.NodeInfo{Addr: joinAddr},
		},
	)
	if err != nil {
		log.Fatal("Can't merge clusters")
	}

	fmt.Println("Cluster merged together")
}
