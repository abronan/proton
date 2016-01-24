package main

import "github.com/codegangsta/cli"

var (
	commands = []cli.Command{
		{
			Name:   "init",
			Usage:  "Initialize a single machine raft cluster",
			Flags:  []cli.Flag{flHosts, flReplication},
			Action: initcluster,
		},
		{
			Name:   "join",
			Usage:  "Join an existing raft cluster",
			Flags:  []cli.Flag{flJoin, flHosts},
			Action: join,
		},
	}
)
