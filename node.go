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

// Handler function can be used and triggered
// everytime there is an append entry event
type Handler func(interface{})

// Type Node represents the Raft Node useful
// configuration.
type Node struct {
	Client  *Proton
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

	// Event is a receive only channel that
	// receives an event when an entry is
	// committed to the logs
	event chan<- struct{}

	// Handler is called when a log entry
	// is committed to the logs, behind can
	// lie anykind of logic processing the
	// message
	handler Handler
}

// Status represents the status of the node
type Status int

const (
	UP Status = iota
	DOWN
	PENDING
)

// Hearbeat regular interval
const hb = 1

// NewNode generates a new Raft node based on an unique
// ID, an address and optionally: a handler and receive
// only channel to send event when en entry is committed
// to the logs
func NewNode(id uint64, addr string, appendEvent chan<- struct{}, handler Handler) *Node {
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
		PStore:  make(map[string]string),
		ticker:  time.Tick(time.Second),
		done:    make(chan struct{}),
		event:   appendEvent,
		handler: handler,
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

// Start is the main loop for a Raft node, it
// goes along the state machine, acting on the
// messages received from other Raft nodes in
// the cluster
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

func (n *Node) Ping(ctx context.Context, ping *PingRequest) (*Acknowledgment, error) {
	return &Acknowledgment{}, nil
}

func (n *Node) JoinCluster(ctx context.Context, info *NodeInfo) (*JoinClusterResponse, error) {
	nodes := []*NodeInfo{}

	for _, node := range n.Cluster.Nodes {
		nodes = append(nodes, &NodeInfo{
			ID:   node.ID,
			Addr: node.Addr,
		})

		if node.ID == n.ID {
			continue
		}

		// Register node on other machines that are part of the cluster
		resp, err := node.Client.Client.AddNode(ctx, info)
		if err != nil || !resp.Success {
			return &JoinClusterResponse{
				Success: false,
				Error:   resp.Error,
			}, nil
		}
	}

	err := n.RegisterNode(info)
	if err != nil {
		return &JoinClusterResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &JoinClusterResponse{
		Success: true,
		Error:   "",
		Info:    nodes,
	}, nil
}

func (n *Node) AddNode(ctx context.Context, info *NodeInfo) (*AddNodeResponse, error) {
	err := n.RegisterNode(info)
	if err != nil {
		return &AddNodeResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &AddNodeResponse{
		Success: true,
		Error:   "",
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

func (n *Node) LeaveRaft(ctx context.Context, info *NodeInfo) (*LeaveRaftResponse, error) {
	confChange := raftpb.ConfChange{
		ID:      info.ID,
		Type:    raftpb.ConfChangeRemoveNode,
		NodeID:  info.ID,
		Context: []byte(""),
	}

	err := n.Raft.ProposeConfChange(n.Ctx, confChange)
	if err != nil {
		return &LeaveRaftResponse{
			Success: false,
			Error:   ErrConfChangeRefused.Error(),
		}, nil
	}

	return &LeaveRaftResponse{
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
	for _, m := range messages {
		// Process locally
		if m.To == n.ID {
			n.Raft.Step(n.Ctx, m)
			continue
		}

		// If node is an active raft member send the message
		if node, ok := n.Cluster.Nodes[m.To]; ok {
			log.Println(raft.DescribeMessage(m, nil))
			_, err := node.Client.Client.Send(n.Ctx, &m)
			if err != nil {
				node.Client.Conn.Close()
				n.Raft.ReportUnreachable(node.ID)
				n.Cluster.RemoveNode(node.ID)
			}
		}
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

		// Send back an event if a channel is defined
		if n.event != nil {
			n.event <- struct{}{}
		}

		// Process a new committed entry if an handler
		// method was defined and provided
		if n.handler != nil {
			n.handler(entry.Data)
		}

		n.PStore[pair.Key] = string(pair.Value)
	}
}

// Register a new node on the cluster
func (n *Node) RegisterNode(node *NodeInfo) error {
	var (
		client *Proton
		err    error
	)

	for i := 1; i <= MaxRetryTime; i++ {
		client, err = GetProtonClient(node.Addr, 2*time.Second)
		if err != nil {
			if i == MaxRetryTime {
				return ErrConnectionRefused
			}
		}
	}

	// Monitor connection
	go func() {
		ticker := time.NewTicker(time.Second * 10)
		for _ = range ticker.C {
			_, err := client.Client.Ping(context.Background(), &PingRequest{})
			if err != nil {
				client.Conn.Close()
				n.Raft.ReportUnreachable(node.ID)
				n.Cluster.RemoveNode(node.ID)
				return
			}
		}
	}()

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

func (n *Node) Leader() bool {
	if n.Raft.Status().Lead == n.ID {
		return true
	}
	return false
}
