package proton

import "sync"

type Cluster struct {
	sync.RWMutex

	Peers map[uint64]*Peer
}

type Peer struct {
	*NodeInfo

	Client *Raft
}

func NewCluster() *Cluster {
	return &Cluster{
		Peers: make(map[uint64]*Peer),
	}
}

// Add a node to our neighbors
func (t *Cluster) AddPeer(peer *Peer) {
	t.Lock()
	t.Peers[peer.ID] = peer
	t.Unlock()
}

// Add multiple nodes to our neighbors
func (t *Cluster) AddPeers(peers []*NodeInfo) {
	t.Lock()
	for _, peer := range peers {
		t.Peers[peer.ID] = &Peer{NodeInfo: peer}
	}
	t.Unlock()
}

//Remove a node from our neighbors
func (t *Cluster) RemovePeer(id uint64) {
	t.Lock()
	delete(t.Peers, id)
	t.Unlock()
}
