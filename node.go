package proton

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
)

var (
	ErrConnectionRefused = errors.New("Connection refused to the node")
)

type Node struct {
	Client  ProtonClient
	Cluster *Cluster
	Ctx     context.Context

	ID        uint64
	PublicIP  string
	PrivateIP string
	Port      int
	Status    Status
	Error     error

	PStore map[string]string
	Store  *raft.MemoryStorage
	Cfg    *raft.Config
	Raft   raft.Node
	ticker <-chan time.Time
	done   <-chan struct{}
}

type Status int

const (
	UP Status = iota
	DOWN
	PENDING
)

const hb = 1

func NewNode(hostname string, peers []raft.Peer) *Node {
	store := raft.NewMemoryStorage()
	h := fnv.New64a()
	h.Write([]byte(hostname))
	id := h.Sum64()
	n := &Node{
		ID:    id,
		Ctx:   context.TODO(),
		Store: store,
		Cfg: &raft.Config{
			ID:              id,
			ElectionTick:    10 * hb,
			HeartbeatTick:   hb,
			Storage:         store,
			MaxSizePerMsg:   math.MaxUint16,
			MaxInflightMsgs: 256,
		},
		PStore: make(map[string]string),
		ticker: time.Tick(time.Second),
		done:   make(chan struct{}),
	}

	n.Raft = raft.StartNode(n.Cfg, peers)
	return n
}

func (n *Node) Start() {
	for {
		select {
		case <-n.ticker:
			n.Raft.Tick()
		case rd := <-n.Raft.Ready():
			n.saveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
			n.send(rd.Messages)
			if !raft.IsEmptySnap(rd.Snapshot) {
				n.processSnapshot(rd.Snapshot)
			}
			for _, entry := range rd.CommittedEntries {
				n.process(entry)
				if entry.Type == raftpb.EntryConfChange {
					var cc raftpb.ConfChange
					cc.Unmarshal(entry.Data)
					n.Raft.ApplyConfChange(cc)
				}
			}
			n.Raft.Advance()
		case <-n.done:
			return
		}
	}
}

func (n *Node) saveToStorage(hardState raftpb.HardState, entries []raftpb.Entry, snapshot raftpb.Snapshot) {
	n.Store.Append(entries)

	if !raft.IsEmptyHardState(hardState) {
		n.Store.SetHardState(hardState)
	}

	if !raft.IsEmptySnap(snapshot) {
		n.Store.ApplySnapshot(snapshot)
	}
}

func (n *Node) send(messages []raftpb.Message) {
	for _, m := range messages {
		log.Println(raft.DescribeMessage(m, nil))

		n.Cluster.Nodes[m.To].Send(n.Ctx, &m)
	}
}

func (n *Node) processSnapshot(snapshot raftpb.Snapshot) {
	panic(fmt.Sprintf("Applying snapshot on node %v is not implemented", n.ID))
}

func (n *Node) process(entry raftpb.Entry) {
	log.Printf("node %v: processing entry: %v\n", n.ID, entry)
	if entry.Type == raftpb.EntryNormal && entry.Data != nil {
		parts := bytes.SplitN(entry.Data, []byte(":"), 2)
		n.PStore[string(parts[0])] = string(parts[1])
	}
}

func (n *Node) receive(ctx context.Context, message raftpb.Message) {
	n.Raft.Step(ctx, message)
}

// Join a new machine in the cluster
func (n *Node) Join(ctx context.Context, in *NodeInfo) (*JoinResponse, error) {
	go n.registerNode(in)

	// TODO propose conf change

	return &JoinResponse{}, nil
}

// Receive message from raft backend
func (n *Node) Send(ctx context.Context, in *raftpb.Message) (*Acknowledgment, error) {

	// TODO receive logic

	return &Acknowledgment{}, nil
}

// Handle Node join event
func (n *Node) registerNode(node *NodeInfo) {
	var (
		client ProtonClient
		err    error
	)

	status := UP

	for i := 1; i <= MaxRetryTime; i++ {
		client, err = GetProtonClient(node.Addr)
		if err != nil {
			if i == MaxRetryTime {
				status = DOWN
			}
		}
	}

	n.Cluster.AddNodes(
		&Node{
			Client: client,
			Status: status,
			Error:  err,
		},
	)
}
