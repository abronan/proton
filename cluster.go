package proton

import "sync"

type Cluster struct {
	sync.RWMutex

	Nodes map[string]*Node
}

func NewCluster() *Cluster {
	return &Cluster{
		Nodes: make(map[string]*Node),
	}
}

// Add a node to our neighbors
func (t *Cluster) AddNodes(node *Node) {
	t.Lock()
	t.Nodes[node.ID] = node
	t.Unlock()
}

// Add a node to our neighbors
func (t *Cluster) RemoveNode(id string) {
	t.Lock()
	delete(t.Nodes, id)
	t.Unlock()
}
