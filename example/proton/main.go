package main

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/codegangsta/cli"
	"github.com/coreos/etcd/raft"
)

var (
	raftLogger = &raft.DefaultLogger{Logger: log.New(os.Stderr, "raft", log.LstdFlags)}
)

func main() {
	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Usage = "Raft etcd with boltdb and grpc"

	app.Authors = []cli.Author{
		{
			Name:  "@abronan",
			Email: "alexandre.beslic@gmail.com",
		},
	}

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:   "debug",
			Usage:  "debug mode",
			EnvVar: "DEBUG",
		},

		cli.StringFlag{
			Name:  "log-level, l",
			Value: "info",
			Usage: fmt.Sprintf("Log level (options: debug, info, warn, error, fatal, panic)"),
		},

		cli.StringFlag{
			Name:  "dbpath",
			Value: "/tmp/proton",
			Usage: "path to boltdb",
		},
	}

	app.Commands = commands

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
