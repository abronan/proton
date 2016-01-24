package proton

import (
	"errors"

	"github.com/satori/go.uuid"
)

var (
	ErrConnectionRefused = errors.New("Connection refused to the node")
)

type Node struct {
	Client ProtonClient

	ID        string
	PublicIP  string
	PrivateIP string
	Port      int
	Status    Status
	Error     error
}

type Status int

const (
	UP Status = iota
	DOWN
	PENDING
)

func GenID() string {
	id := uuid.NewV4()
	return id.String()
}

func NewNode(client ProtonClient) *Node {
	return &Node{
		ID:     GenID(),
		Client: client,
	}
}
