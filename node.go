package proton

import (
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/gogo/protobuf/proto"
)

var (
	ErrConnectionRefused = errors.New("Connection refused to the node")
	ErrConfChangeRefused = errors.New("Can't add node to the cluster")
)

type Node struct {
	Client  ProtonClient
	Cluster *Cluster
	Ctx     context.Context

	ID     uint64
	Addr   string
	Port   int
	Status Status
	Error  error

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

func NewNode(id uint64, addr string) *Node {
	store := raft.NewMemoryStorage()
	peers := []raft.Peer{{ID: id}}

	n := &Node{
		ID:      id,
		Ctx:     context.TODO(),
		Cluster: NewCluster(),
		Store:   store,
		Addr:    addr,
		Cfg: &raft.Config{
			ID:              id,
			ElectionTick:    5 * hb,
			HeartbeatTick:   hb,
			Storage:         store,
			MaxSizePerMsg:   math.MaxUint16,
			MaxInflightMsgs: 256,
		},
		PStore: make(map[string]string),
		ticker: time.Tick(time.Second),
		done:   make(chan struct{}),
	}

	n.Cluster.AddNodes(
		&Node{
			ID:   id,
			Addr: addr,
		},
	)

	n.Raft = raft.StartNode(n.Cfg, peers)
	return n
}

func GenID(hostname string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(hostname))
	return h.Sum64()
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

func (n *Node) JoinCluster(ctx context.Context, info *NodeInfo) (*JoinClusterResponse, error) {
	err := n.RegisterNode(info)
	if err != nil {
		return &JoinClusterResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	nodes := []*NodeInfo{}
	for _, node := range n.Cluster.Nodes {
		nodes = append(nodes, &NodeInfo{
			ID:   node.ID,
			Addr: node.Addr,
		})
	}

	return &JoinClusterResponse{
		Success: true,
		Error:   "",
		Info:    nodes,
	}, nil
}

func (n *Node) JoinRaft(ctx context.Context, info *NodeInfo) (*JoinRaftResponse, error) {
	confChange := raftpb.ConfChange{
		ID:      info.ID,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  info.ID,
		Context: []byte(""),
	}

	err := n.Raft.ProposeConfChange(n.Ctx, confChange)
	if err != nil {
		return &JoinRaftResponse{
			Success: false,
			Error:   ErrConfChangeRefused.Error(),
		}, nil
	}

	return &JoinRaftResponse{
		Success: true,
		Error:   "",
	}, nil
}

func (n *Node) Send(ctx context.Context, message *raftpb.Message) (*Acknowledgment, error) {
	n.Raft.Step(n.Ctx, *message)

	return &Acknowledgment{}, nil
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
	for key, _ := range n.Cluster.Nodes {
		fmt.Println("node value in cluster list: ", key)
	}

	for _, m := range messages {
		// Process locally
		if m.To == n.ID {
			n.Raft.Step(n.Ctx, m)
		}

		log.Println(raft.DescribeMessage(m, nil))

		n.Cluster.Nodes[m.To].Client.Send(n.Ctx, &m)
	}
}

func (n *Node) processSnapshot(snapshot raftpb.Snapshot) {
	panic(fmt.Sprintf("Applying snapshot on node %v is not implemented", n.ID))
}

func (n *Node) process(entry raftpb.Entry) {
	log.Printf("node %v: processing entry: %v\n", n.ID, entry)
	if entry.Type == raftpb.EntryNormal && entry.Data != nil {
		pair := &Pair{}
		err := proto.Unmarshal(entry.Data, pair)
		if err != nil {
			log.Fatal("Can't decode key and value sent through raft")
		}

		n.PStore[pair.Key] = string(pair.Value)
	}
}

// Register a new node on the cluster
func (n *Node) RegisterNode(node *NodeInfo) error {
	var (
		client ProtonClient
		err    error
	)

	for i := 1; i <= MaxRetryTime; i++ {
		client, err = GetProtonClient(node.Addr)
		if err != nil {
			if i == MaxRetryTime {
				return ErrConnectionRefused
			}
		}
	}

	// TODO monitor connection

	n.Cluster.AddNodes(
		&Node{
			ID:     node.ID,
			Addr:   node.Addr,
			Client: client,
			Error:  err,
		},
	)

	return nil
}
