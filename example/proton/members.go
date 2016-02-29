package main

import (
	"fmt"
	"log"
	"time"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
	"golang.org/x/net/context"
)

func members(c *cli.Context) {
	var (
		err error
	)

	hosts := c.StringSlice("host")
	if c.IsSet("host") || c.IsSet("H") {
		hosts = hosts[1:]
	}

	client, err := proton.GetRaftClient(hosts[0], 2*time.Second)
	if err != nil {
		log.Fatal("couldn't initialize client connection")
	}

	resp, err := client.ListMembers(context.TODO(), &proton.ListMembersRequest{})
	if err != nil {
		log.Fatal("Can't list members in the cluster")
	}

	fmt.Println("Nodes:")

	for _, node := range resp.Members {
		fmt.Println(":", node.ID)
	}
}
