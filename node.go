package proton

import (
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net"
	"time"

	"google.golang.org/grpc"

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

// Type RaftNode represents the Raft Node useful
// configuration.
type RaftNode struct {
	raft.Node

	Client   *Raft
	Cluster  *Cluster
	Server   *grpc.Server
	Listener net.Listener
	Ctx      context.Context

	ID    uint64
	Addr  string
	Port  int
	Error error

	PStore map[string]string
	Store  *raft.MemoryStorage
	Cfg    *raft.Config

	transport *Transport
	ticker    <-chan time.Time
	done      <-chan struct{}
	stopChan  chan struct{}
	pauseChan chan bool
	paused    bool
	rcvmsg    []raftpb.Message
	debug     bool

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

// TODO implement clean Transport interface?
type Transport interface {
}

// NewRaftNode generates a new Raft node based on an unique
// ID, an address and optionally: a handler and receive
// only channel to send event when en entry is committed
// to the logs
func NewRaftNode(id uint64, addr string, heartbeat int, server *grpc.Server, listener net.Listener, debug bool, appendEvent chan<- struct{}, handler Handler) *RaftNode {
	store := raft.NewMemoryStorage()
	peers := []raft.Peer{{ID: id}}

	n := &RaftNode{
		ID:       id,
		Ctx:      context.TODO(),
		Cluster:  NewCluster(),
		Server:   server,
		Listener: listener,
		Store:    store,
		Addr:     addr,
		Cfg: &raft.Config{
			ID:              id,
			ElectionTick:    5 * heartbeat,
			HeartbeatTick:   heartbeat,
			Storage:         store,
			MaxSizePerMsg:   math.MaxUint16,
			MaxInflightMsgs: 256,
		},
		PStore:    make(map[string]string),
		ticker:    time.Tick(time.Second),
		done:      make(chan struct{}),
		stopChan:  make(chan struct{}),
		pauseChan: make(chan bool),
		event:     appendEvent,
		handler:   handler,
		debug:     debug,
	}

	n.Cluster.AddPeer(
		&Peer{
			NodeInfo: &NodeInfo{
				ID:   id,
				Addr: addr,
			},
		},
	)

	n.Node = raft.StartNode(n.Cfg, peers)
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
func (n *RaftNode) Start() {
	for {
		select {
		case <-n.ticker:
			n.Tick()

		case rd := <-n.Ready():
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
					switch cc.Type {
					case raftpb.ConfChangeAddNode:
						peer, err := unmarshalNodeInfo(cc.Context)
						if err != nil {
							continue
						}
						if n.ID != peer.ID {
							n.RegisterNode(peer)
						}
					case raftpb.ConfChangeRemoveNode:
						n.UnregisterNode(cc.NodeID)
						n.ReportUnreachable(cc.NodeID)
					default:
					}
					n.ApplyConfChange(cc)
				}
			}
			n.Advance()

		case <-n.stopChan:
			n.Stop()
			n.Node = nil
			close(n.stopChan)
			return

		case pause := <-n.pauseChan:
			n.paused = pause
			n.rcvmsg = make([]raftpb.Message, 0)
			for pause {
				select {
				case pause = <-n.pauseChan:
					n.paused = pause
				}
			}
			// process pending messages
			for _, m := range n.rcvmsg {
				n.Step(context.TODO(), m)
			}
			n.rcvmsg = nil

		case <-n.done:
			return
		}
	}
}

// JoinRaft sends a configuration change to nodes to
// add a new member to the raft cluster
func (n *RaftNode) JoinRaft(ctx context.Context, info *NodeInfo) (*JoinRaftResponse, error) {
	meta, err := proto.Marshal(info)
	if err != nil {
		log.Fatal("Can't marshal node: ", info.ID)
	}

	confChange := raftpb.ConfChange{
		ID:      info.ID,
		Type:    raftpb.ConfChangeAddNode,
		NodeID:  info.ID,
		Context: meta,
	}

	err = n.ProposeConfChange(n.Ctx, confChange)
	if err != nil {
		return &JoinRaftResponse{
			Success: false,
			Error:   ErrConfChangeRefused.Error(),
		}, nil
	}

	nodes := make([]*NodeInfo, 0)
	for _, node := range n.Cluster.Peers {
		nodes = append(nodes, &NodeInfo{
			ID:   node.ID,
			Addr: node.Addr,
		})
	}

	return &JoinRaftResponse{
		Success: true,
		Nodes:   nodes,
		Error:   "",
	}, nil
}

// LeaveRaft sends a configuration change for a node
// that is willing to abandon its raft cluster membership
func (n *RaftNode) LeaveRaft(ctx context.Context, info *NodeInfo) (*LeaveRaftResponse, error) {
	confChange := raftpb.ConfChange{
		ID:      info.ID,
		Type:    raftpb.ConfChangeRemoveNode,
		NodeID:  info.ID,
		Context: []byte(""),
	}

	err := n.ProposeConfChange(n.Ctx, confChange)
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

// Send calls 'Step' which advances the raft state
// machine with the received message
func (n *RaftNode) Send(ctx context.Context, msg *raftpb.Message) (*SendResponse, error) {
	var err error

	if n.paused {
		n.rcvmsg = append(n.rcvmsg, *msg)
	} else {
		err = n.Step(n.Ctx, *msg)
		if err != nil {
			return &SendResponse{Error: err.Error()}, nil
		}
	}

	return &SendResponse{Error: ""}, nil
}

// Saves a log entry to our Store
func (n *RaftNode) saveToStorage(hardState raftpb.HardState, entries []raftpb.Entry, snapshot raftpb.Snapshot) {
	n.Store.Append(entries)

	if !raft.IsEmptyHardState(hardState) {
		n.Store.SetHardState(hardState)
	}

	if !raft.IsEmptySnap(snapshot) {
		n.Store.ApplySnapshot(snapshot)
	}
}

// Sends a series of messages to members in the raft
func (n *RaftNode) send(messages []raftpb.Message) {
	for _, m := range messages {
		// Process locally
		if m.To == n.ID {
			n.Step(n.Ctx, m)
			continue
		}

		// If node is an active raft member send the message
		if peer, ok := n.Cluster.Peers[m.To]; ok {
			if n.debug {
				log.Println(raft.DescribeMessage(m, nil))
			}
			_, err := peer.Client.Send(n.Ctx, &m)
			if err != nil {
				if err := n.RemoveNode(peer); err != nil {
					log.Println("Cannot report node unreachable for ID: ", peer.ID)
				}
			}
		}
	}
}

// Process snapshot is not yet implemented but applies
// a snapshot to handle node failures and restart
func (n *RaftNode) processSnapshot(snapshot raftpb.Snapshot) {
	// TODO
	panic(fmt.Sprintf("Applying snapshot on node %v is not implemented", n.ID))
}

// Process a data entry and optionnally triggers an event
// or a function handler after the entry is processed
func (n *RaftNode) process(entry raftpb.Entry) {
	if n.debug {
		log.Printf("node %v: processing entry: %v\n", n.ID, entry)
	}

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

func (n *RaftNode) RemoveNode(node *Peer) error {
	confChange := raftpb.ConfChange{
		ID:      node.ID,
		Type:    raftpb.ConfChangeRemoveNode,
		NodeID:  node.ID,
		Context: []byte(""),
	}

	err := n.ProposeConfChange(n.Ctx, confChange)
	if err != nil {
		return err
	}
	return nil
}

// RegisterNode registers a new node on the cluster
func (n *RaftNode) RegisterNode(node *NodeInfo) error {
	var (
		client *Raft
		err    error
	)

	for i := 1; i <= MaxRetryTime; i++ {
		client, err = GetRaftClient(node.Addr, 2*time.Second)
		if err != nil {
			if i == MaxRetryTime {
				return ErrConnectionRefused
			}
		}
	}

	n.Cluster.AddPeer(
		&Peer{
			NodeInfo: node,
			Client:   client,
		},
	)

	return nil
}

// RegisterNodes registers a set of nodes in the cluster
func (n *RaftNode) RegisterNodes(nodes []*NodeInfo) error {
	for _, node := range nodes {
		n.RegisterNode(node)
	}

	return nil
}

// UnregisterNode unregisters a node that has died or
// has gracefully left the raft subsystem
func (n *RaftNode) UnregisterNode(id uint64) {
	// Do not unregister yourself
	if n.ID == id {
		return
	}

	n.Cluster.Peers[id].Client.Conn.Close()
	n.Cluster.RemovePeer(id)
}

// IsLeader checks if we are the leader or not
func (n *RaftNode) IsLeader() bool {
	if n.Node.Status().Lead == n.ID {
		return true
	}
	return false
}

// Leader returns the id of the leader
func (n *RaftNode) Leader() uint64 {
	return n.Node.Status().Lead
}

func unmarshalNodeInfo(nodeInfo []byte) (*NodeInfo, error) {
	info := &NodeInfo{}
	err := proto.Unmarshal(nodeInfo, info)
	if err != nil {
		return nil, err
	}
	return info, nil
}
