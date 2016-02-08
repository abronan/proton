package main

import "github.com/codegangsta/cli"

var (
	flHostsValue = cli.StringSlice([]string{"127.0.0.1:6744"})

	flHosts = cli.StringSliceFlag{
		Name:   "host, H",
		Value:  &flHostsValue,
		Usage:  "ip/socket to listen on",
		EnvVar: "PROTON_HOST",
	}

	flFirst = cli.BoolFlag{
		Name:   "first",
		Usage:  "be the first node for topology construction; implied by --first=true",
		EnvVar: "PROTON_FIRST",
	}

	flWithRaftLogs = cli.BoolFlag{
		Name:   "withRaftLogs",
		Usage:  "print raft logs for heartbeats and log processing",
		EnvVar: "PROTON_WITH_RAFT_LOGS",
	}

	flReplication = cli.BoolFlag{
		Name:   "replication, R",
		Usage:  "take part in log replication; implied by --replication=true",
		EnvVar: "PROTON_LOG_REPLICATION",
	}

	flJoin = cli.StringFlag{
		Name:   "join, J",
		Usage:  "ip/socket to join",
		EnvVar: "PROTON_JOIN",
	}

	flHostname = cli.StringFlag{
		Name:   "hostname",
		Usage:  "hostname of the raft node",
		EnvVar: "PROTON_HOSTNAME",
	}
)
