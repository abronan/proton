package main

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/net/context"

	"github.com/abronan/proton"
	"github.com/codegangsta/cli"
)

func list(c *cli.Context) {
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

	resp, err := client.ListObjects(context.TODO(), &proton.ListObjectsRequest{})
	if err != nil {
		log.Fatal("Can't list objects in the cluster")
	}

	fmt.Println("Keys:")

	for _, obj := range resp.Objects {
		fmt.Println(":", obj.Key, ":", string(obj.Value))
	}
}
