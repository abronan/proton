package main

import "github.com/codegangsta/cli"

var (
	commands = []cli.Command{
		{
			Name:   "init",
			Usage:  "Initialize a single machine raft cluster",
			Flags:  []cli.Flag{flHosts, flReplication, flHostname, flWithRaftLogs},
			Action: initcluster,
		},
		{
			Name:   "join",
			Usage:  "Join an existing raft cluster",
			Flags:  []cli.Flag{flJoin, flHosts, flHostname, flWithRaftLogs},
			Action: join,
		},
		{
			Name:   "put",
			Usage:  "Put a value on the raft store",
			Flags:  []cli.Flag{flHosts, flKey, flValue},
			Action: put,
		},
		{
			Name:   "list",
			Usage:  "List values in the raft store",
			Flags:  []cli.Flag{flHosts},
			Action: list,
		},
		{
			Name:   "members",
			Usage:  "List the members of the raft cluster",
			Flags:  []cli.Flag{flHosts},
			Action: members,
		},
	}
)
