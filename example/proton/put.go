package main

import (
	"log"
	"time"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
	"golang.org/x/net/context"
)

func put(c *cli.Context) {
	var (
		err error
	)

	hosts := c.StringSlice("host")
	if c.IsSet("host") || c.IsSet("H") {
		hosts = hosts[1:]
	}

	key := c.String("key")
	if key == "" {
		log.Fatal("key flag must be set")
	}

	value := []byte(c.String("value"))
	if c.String("value") == "" {
		log.Fatal("value flag must be set")
	}

	client, err := proton.GetRaftClient(hosts[0], 2*time.Second)
	if err != nil {
		log.Fatal("couldn't initialize client connection")
	}

	pair := &proton.Proposal_Pair{
		&proton.Pair{Key: key, Value: value},
	}
	proposal := &proton.Proposal{
		Proposal: pair,
	}

	resp, err := client.PutObject(context.TODO(), &proton.PutObjectRequest{proposal})
	if resp == nil || err != nil {
		log.Fatal("Can't put object in the cluster")
	}
}
