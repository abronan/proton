package proton

import (
	"errors"
	"hash/fnv"
	"log"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/gogo/protobuf/proto"
)

var (
	defaultLogger = &raft.DefaultLogger{Logger: log.New(os.Stderr, "raft", log.LstdFlags)}

	// ErrConnectionRefused is thrown when a connection is refused to a node member in the raft
	ErrConnectionRefused = errors.New("connection refused to the node")
	// ErrConfChangeRefused is thrown when there is an issue with the configuration change
	ErrConfChangeRefused = errors.New("propose configuration change refused")
	// ErrApplyNotSpecified is thrown during the creation of a raft node when no apply method was provided
	ErrApplyNotSpecified = errors.New("apply method was not specified")
	// ErrRaftMerging is thrown when we try to send a message to a node that is in the middle of a merge process
	ErrRaftMerging = errors.New("can't send message, raft node in the middle of merge process")
)

// ApplyCommand function can be used and triggered
// every time there is an append entry event
type ApplyCommand func(interface{})

// Node represents the Raft Node useful
// configuration.
type Node struct {
	raft.Node

	Client   *Raft
	Cluster  *Cluster
	Server   *grpc.Server
	Listener net.Listener
	Ctx      context.Context

	ID      uint64
	Address string
	Port    int
	Error   error

	storeLock sync.RWMutex
	PStore    map[string][]byte
	Store     *raft.MemoryStorage
	Cfg       *raft.Config

	ticker    *time.Ticker
	stopChan  chan struct{}
	pauseChan chan bool
	pauseLock sync.RWMutex
	pause     bool
	mergeLock sync.RWMutex
	merge     bool
	rcvmsg    []raftpb.Message
	errCh     chan error

	// ApplyCommand is called when a log entry
	// is committed to the logs, behind can
	// lie any kind of logic processing the
	// message
	apply ApplyCommand
}

// NewNode generates a new Raft node based on an unique
// ID, an address and optionally: a handler and receive
// only channel to send event when an entry is committed
// to the logs
func NewNode(id uint64, addr string, cfg *raft.Config, apply ApplyCommand) (*Node, error) {
	if cfg == nil {
		cfg = DefaultNodeConfig()
	}

	store := raft.NewMemoryStorage()
	peers := []raft.Peer{{ID: id}}

	n := &Node{
		ID:      id,
		Ctx:     context.TODO(),
		Cluster: NewCluster(),
		Store:   store,
		Address: addr,
		Cfg: &raft.Config{
			ID:              id,
			ElectionTick:    cfg.ElectionTick,
			HeartbeatTick:   cfg.HeartbeatTick,
			Storage:         store,
			MaxSizePerMsg:   cfg.MaxSizePerMsg,
			MaxInflightMsgs: cfg.MaxInflightMsgs,
			Logger:          cfg.Logger,
		},
		PStore:    make(map[string][]byte),
		ticker:    time.NewTicker(time.Second),
		stopChan:  make(chan struct{}),
		pauseChan: make(chan bool),
		apply:     apply,
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
	return n, nil
}

// DefaultNodeConfig returns the default config for a
// raft node that can be modified and customized
func DefaultNodeConfig() *raft.Config {
	return &raft.Config{
		HeartbeatTick:   1,
		ElectionTick:    3,
		MaxSizePerMsg:   math.MaxUint16,
		MaxInflightMsgs: 256,
		Logger:          defaultLogger,
	}
}

// GenID generate an id for a raft node
// given a hostname.
//
// FIXME there is a high chance of id collision
func GenID(hostname string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(hostname))
	return h.Sum64()
}

// Start is the main loop for a Raft node, it
// goes along the state machine, acting on the
// messages received from other Raft nodes in
// the cluster
func (n *Node) Start() (errCh <-chan error) {
	n.errCh = make(chan error)
	go func() {
		for {
			select {
			case <-n.ticker.C:
				n.Tick()

			case rd := <-n.Ready():
				if n.isMerging() {
					continue
				}
				n.saveToStorage(rd.HardState, rd.Entries, rd.Snapshot)
				n.send(rd.Messages)
				if !raft.IsEmptySnap(rd.Snapshot) {
					n.processSnapshot(rd.Snapshot)
				}
				for _, entry := range rd.CommittedEntries {
					err := n.process(entry)
					if err != nil {
						n.errCh <- err
					}
					if entry.Type == raftpb.EntryConfChange {
						var cc raftpb.ConfChange
						err := cc.Unmarshal(entry.Data)
						if err != nil {
							n.errCh <- err
						}
						switch cc.Type {
						case raftpb.ConfChangeAddNode:
							n.applyAddNode(cc)
						case raftpb.ConfChangeRemoveNode:
							n.applyRemoveNode(cc)
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
				// FIXME lock hell
				n.SetPaused(pause)
				for n.pause {
					select {
					case pause = <-n.pauseChan:
						n.SetPaused(pause)
					}
				}
				n.pauseLock.Lock()
				// process pending messages
				for _, m := range n.rcvmsg {
					err := n.Step(n.Ctx, m)
					if err != nil {
						log.Fatal("Something went wrong when unpausing the node")
					}
				}
				n.rcvmsg = nil
				n.pauseLock.Unlock()
			}
		}
	}()
	return n.errCh
}

// Shutdown stops the raft node processing loop.
// Calling Shutdown on an already stopped node
// will result in a deadlock
func (n *Node) Shutdown() {
	n.stopChan <- struct{}{}
}

// Pause pauses the raft node
func (n *Node) Pause() {
	n.pauseChan <- true
}

// Resume brings back the raft node to activity
func (n *Node) Resume() {
	n.pauseChan <- false
}

// IsPaused checks if a node is paused or not
func (n *Node) IsPaused() bool {
	n.pauseLock.Lock()
	defer n.pauseLock.Unlock()
	return n.pause
}

// SetPaused sets the switch for the pause mode
func (n *Node) SetPaused(pause bool) {
	n.pauseLock.Lock()
	defer n.pauseLock.Unlock()
	n.pause = pause
	if n.rcvmsg == nil {
		n.rcvmsg = make([]raftpb.Message, 0)
	}
}

// IsLeader checks if we are the leader or not
func (n *Node) IsLeader() bool {
	if n.Node.Status().Lead == n.ID {
		return true
	}
	return false
}

// Leader returns the id of the leader
func (n *Node) Leader() uint64 {
	return n.Node.Status().Lead
}

// JoinRaft sends a configuration change to nodes to
// add a new member to the raft cluster
func (n *Node) JoinRaft(ctx context.Context, info *NodeInfo) (*JoinRaftResponse, error) {
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
		return nil, err
	}

	var nodes []*NodeInfo
	for _, node := range n.Cluster.Peers() {
		nodes = append(nodes, &NodeInfo{
			ID:   node.ID,
			Addr: node.Addr,
		})
	}

	return &JoinRaftResponse{Nodes: nodes}, nil
}

// LeaveRaft sends a configuration change for a node
// that is willing to abandon its raft cluster membership
func (n *Node) LeaveRaft(ctx context.Context, info *NodeInfo) (*LeaveRaftResponse, error) {
	confChange := raftpb.ConfChange{
		ID:      info.ID,
		Type:    raftpb.ConfChangeRemoveNode,
		NodeID:  info.ID,
		Context: []byte(""),
	}

	err := n.ProposeConfChange(n.Ctx, confChange)
	if err != nil {
		return nil, err
	}

	return &LeaveRaftResponse{}, nil
}

// Send calls 'Step' which advances the raft state
// machine with the received message
func (n *Node) Send(ctx context.Context, msg *raftpb.Message) (*SendResponse, error) {
	var err error

	if n.IsPaused() {
		n.pauseLock.Lock()
		n.rcvmsg = append(n.rcvmsg, *msg)
		n.pauseLock.Unlock()
	} else {
		if n.isMerging() {
			return nil, ErrRaftMerging
		}
		err = n.Step(n.Ctx, *msg)
		if err != nil {
			return nil, err
		}
	}

	return &SendResponse{}, nil
}

// ListMembers lists the members in the raft cluster
func (n *Node) ListMembers(ctx context.Context, req *ListMembersRequest) (*ListMembersResponse, error) {
	var peers []*NodeInfo
	for _, peer := range n.Cluster.Peers() {
		peers = append(peers, peer.NodeInfo)
	}

	return &ListMembersResponse{Members: peers}, nil
}

// Put proposes and puts a value in the raft cluster
func (n *Node) PutObject(ctx context.Context, req *PutObjectRequest) (*PutObjectResponse, error) {
	prop, err := proto.Marshal(req.Proposal)
	if err != nil {
		return nil, err
	}

	err = n.Propose(n.Ctx, prop)
	if err != nil {
		return nil, err
	}

	return &PutObjectResponse{}, nil
}

// ListObjects list the objects in the raft cluster
func (n *Node) ListObjects(ctx context.Context, req *ListObjectsRequest) (*ListObjectsResponse, error) {
	pairs := n.ListPairs()

	return &ListObjectsResponse{Objects: pairs}, nil
}

// StopRaft is generally used from a coordinator node in the
// raft to initiate or control a merge or split process
func (n *Node) StopRaft(ctx context.Context, req *StopRaftRequest) (*StopRaftResponse, error) {
	n.Stop()
	return &StopRaftResponse{}, nil
}

// Merge initiates the process of merging two raft
// clusters together, it calls MergeInit() on every
// node affected by the operation
func (n *Node) Merge(ctx context.Context, req *MergeRequest) (*MergeResponse, error) {
	for _, m := range n.Cluster.Peers() {
		_, err := m.Client.MergeInit(n.Ctx, &MergeInitRequest{})
		if err != nil {
			return nil, err
		}
	}

	return &MergeResponse{}, nil
}

// MergeInit initiates the process of merging two raft
// clusters together, it is called from a node receiving
// the request that the merging process has started, it is
// used to notify everyone
func (n *Node) MergeInit(ctx context.Context, req *MergeInitRequest) (*MergeInitResponse, error) {
	var err error

	// Stop the current raft node
	n.initMerge()
	n.closeConn()
	n.Shutdown()
	pairs := n.PStore

	// Init a new Raft Node
	cfg := DefaultNodeConfig()
	cfg.Logger = defaultLogger
	n, err = NewNode(n.ID, n.Address, cfg, n.apply)
	if err != nil {
		return nil, err
	}

	// Start the new raft node
	n.openConn()
	n.Start()

	// Join the new cluster
	client, err := GetRaftClient(req.Nodes[0].Addr, 2*time.Second)
	if err != nil {
		return nil, err
	}

	info := &NodeInfo{ID: n.ID, Addr: n.Address}
	resp, err := client.JoinRaft(context.Background(), info)
	if err != nil {
		return nil, err
	}

	if err = n.RegisterNodes(resp.GetNodes()); err != nil {
		return nil, err
	}

	// If leader, propose diff and finalize merge
	if n.IsLeader() {
		var objects []*Pair
		for k, v := range pairs {
			objects = append(objects, &Pair{Key: k, Value: v})
		}

		proposal, err := EncodeDiff(objects)
		if err != nil {
			return nil, err
		}

		err = n.Propose(n.Ctx, proposal)
		if err != nil {
			return nil, err
		}

		for _, node := range n.Cluster.Peers() {
			if node.ID == n.ID {
				continue
			}
			_, err := node.Client.MergeFinalize(n.Ctx, &MergeFinalizeRequest{})
			if err != nil {
				return nil, err
			}
		}

		n.finalizeMerge()
	}

	return &MergeInitResponse{}, nil
}

// MergeFinalize finalizes the process of merging two raft
// clusters together, it is called from the coordinator node
// managing the merge process
func (n *Node) MergeFinalize(ctx context.Context, req *MergeFinalizeRequest) (*MergeFinalizeResponse, error) {
	n.finalizeMerge()
	return &MergeFinalizeResponse{}, nil
}

// RemoveNode removes a node from the raft cluster
func (n *Node) RemoveNode(node *Peer) error {
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
func (n *Node) RegisterNode(node *NodeInfo) error {
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
func (n *Node) RegisterNodes(nodes []*NodeInfo) (err error) {
	for _, node := range nodes {
		err = n.RegisterNode(node)
		if err != nil {
			return err
		}
	}

	return nil
}

// UnregisterNode unregisters a node that has died or
// has gracefully left the raft subsystem
func (n *Node) UnregisterNode(id uint64) {
	// Do not unregister yourself
	if n.ID == id {
		return
	}

	n.Cluster.Peers()[id].Client.Conn.Close()
	n.Cluster.RemovePeer(id)
}

// Get returns a value from the PStore
func (n *Node) Get(key string) []byte {
	n.storeLock.RLock()
	defer n.storeLock.RUnlock()
	return n.PStore[key]
}

// Put puts a value in the raft store
func (n *Node) Put(key string, value []byte) {
	n.storeLock.Lock()
	defer n.storeLock.Unlock()
	n.PStore[key] = value
}

// List lists the pair in the store
func (n *Node) ListPairs() []*Pair {
	n.storeLock.Lock()
	defer n.storeLock.Unlock()
	var pairs []*Pair
	for k, v := range n.PStore {
		pairs = append(pairs, &Pair{Key: k, Value: []byte(v)})
	}
	return pairs
}

// StoreLength returns the length of the store
func (n *Node) StoreLength() int {
	n.storeLock.Lock()
	defer n.storeLock.Unlock()
	return len(n.PStore)
}

// applyAddNode is called when we receive a ConfChange
// from a member in the raft cluster, this adds a new
// node to the existing raft cluster
func (n *Node) applyAddNode(conf raftpb.ConfChange) error {
	peer := &NodeInfo{}
	err := proto.Unmarshal(conf.Context, peer)
	if err != nil {
		return err
	}
	if n.ID != peer.ID {
		n.RegisterNode(peer)
	}
	return nil
}

// applyRemoveNode is called when we receive a ConfChange
// from a member in the raft cluster, this removes a node
// from the existing raft cluster
func (n *Node) applyRemoveNode(conf raftpb.ConfChange) {
	// The leader steps down
	if n.ID == n.Leader() && n.ID == conf.NodeID {
		n.Stop()
		return
	}
	// If a follower and the leader steps
	// down, Campaign to be the leader
	if conf.NodeID == n.Leader() {
		n.Campaign(n.Ctx)
	}
	n.UnregisterNode(conf.NodeID)
}

// Saves a log entry to our Store
func (n *Node) saveToStorage(hardState raftpb.HardState, entries []raftpb.Entry, snapshot raftpb.Snapshot) {
	n.Store.Append(entries)

	if !raft.IsEmptyHardState(hardState) {
		n.Store.SetHardState(hardState)
	}

	if !raft.IsEmptySnap(snapshot) {
		n.Store.ApplySnapshot(snapshot)
	}
}

// Sends a series of messages to members in the raft
func (n *Node) send(messages []raftpb.Message) {
	peers := n.Cluster.Peers()

	for _, m := range messages {
		// Process locally
		if m.To == n.ID {
			n.Step(n.Ctx, m)
			continue
		}

		// If node is an active raft member send the message
		if peer, ok := peers[m.To]; ok {
			_, err := peer.Client.Send(n.Ctx, &m)
			if err != nil {
				n.ReportUnreachable(peer.ID)
			}
		}
	}
}

// Process a data entry and optionnally triggers an event
// or a function handler after the entry is processed
func (n *Node) process(entry raftpb.Entry) error {
	if entry.Type == raftpb.EntryNormal && entry.Data != nil {
		p := &Proposal{}
		err := proto.Unmarshal(entry.Data, p)
		if err != nil {
			return err
		}

		// Apply the command
		if n.apply != nil {
			n.apply(entry.Data)
		}

		// Put the value(s) into the store
		switch prop := p.GetProposal().(type) {
		case *Proposal_Pair:
			n.Put(prop.Pair.Key, prop.Pair.Value)
		case *Proposal_Diff:
			for _, pair := range prop.Diff.GetPairs() {
				n.Put(pair.Key, pair.Value)
			}
		}
	}
	return nil
}

// Process snapshot is not yet implemented but applies
// a snapshot to handle node failures and restart
func (n *Node) processSnapshot(snapshot raftpb.Snapshot) error {
	return n.Store.ApplySnapshot(snapshot)
}

// Take snapshot takes a snapshot of the current state
func (n *Node) takeSnapshot() error {
	return nil
}

// Closes the connections for the raft node
func (n *Node) closeConn() {
	n.Server.Stop()
	n.Server.TestingCloseConns()
	n.Listener.Close()
	n.Listener = nil

	// FIXME need to wait for the connections
	// to be cleaned up properly
	time.Sleep(2 * time.Second)
}

// Opens up the connections to the raft node
func (n *Node) openConn() error {
	l, err := net.Listen("tcp", n.Address)
	if err != nil {

	}
	s := grpc.NewServer()
	Register(s, n)

	go s.Serve(l)

	n.Listener = l
	n.Server = s
	return nil
}

func (n *Node) initMerge() {
	n.pauseLock.Lock()
	n.pause = true
	n.pauseLock.Unlock()
}

func (n *Node) finalizeMerge() {
	n.pauseLock.Lock()
	n.pause = false
	n.pauseLock.Unlock()
}

func (n *Node) isMerging() bool {
	n.pauseLock.Lock()
	defer n.pauseLock.Unlock()
	return n.pause
}
