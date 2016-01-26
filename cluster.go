package proton

import "sync"

type Cluster struct {
	sync.RWMutex

	Nodes map[uint64]*Node
}

func NewCluster() *Cluster {
	return &Cluster{
		Nodes: make(map[uint64]*Node),
	}
}

// Add a node to our neighbors
func (t *Cluster) AddNodes(node *Node) {
	t.Lock()
	t.Nodes[node.ID] = node
	t.Unlock()
}

// Add a node to our neighbors
func (t *Cluster) RemoveNode(id uint64) {
	t.Lock()
	delete(t.Nodes, id)
	t.Unlock()
}
