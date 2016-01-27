package main

import (
	"fmt"
	"time"

	"github.com/abronan/proton"
	"github.com/coreos/etcd/raft/raftpb"
)

var (
	nodes = make(map[int]*proton.Node)
)

func main() {
	id := proton.GenID("Host1")
	nodes[1] = proton.NewNode(id)
	nodes[1].Raft.Campaign(nodes[1].Ctx)
	go nodes[1].Start()

	id = proton.GenID("Host2")
	nodes[2] = proton.NewNode(id)
	go nodes[2].Start()

	id = proton.GenID("Host3")
	nodes[3] = proton.NewNode(id)
	go nodes[3].Start()
	nodes[2].Raft.ProposeConfChange(nodes[2].Ctx, raftpb.ConfChange{
		ID:      3,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  3,
		Context: []byte(""),
	})

	// Wait for leader, is there a better way to do this
	for nodes[1].Raft.Status().Lead != 1 {
		time.Sleep(100 * time.Millisecond)
	}

	nodes[1].Raft.Propose(nodes[1].Ctx, []byte("mykey1:myvalue1"))
	nodes[2].Raft.Propose(nodes[2].Ctx, []byte("mykey2:myvalue2"))
	nodes[3].Raft.Propose(nodes[3].Ctx, []byte("mykey3:myvalue3"))

	// Wait for proposed entry to be commited in cluster.
	// Apperently when should add an uniq id to the message and wait until it is
	// commited in the node.
	fmt.Printf("** Sleeping to visualize heartbeat between nodes **\n")
	time.Sleep(2000 * time.Millisecond)

	// Just check that data has been persited
	for i, node := range nodes {
		fmt.Printf("** Node %v **\n", i)
		for k, v := range node.PStore {
			fmt.Printf("%v = %v\n", k, v)
		}
		fmt.Printf("*************\n")
	}
}
