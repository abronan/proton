package proton

import (
	"net"
	"time"

	"google.golang.org/grpc"

	"golang.org/x/net/context"
)

const (
	MaxRetryTime = 3
)

func GetProtonClient(addr string) (ProtonClient, error) {
	conn, err := getClientConn(addr, "tcp")
	if err != nil {
		return nil, err
	}
	return NewProtonClient(conn), nil
}

func getClientConn(addr string, protocol string) (*grpc.ClientConn, error) {
	dialOpts := []grpc.DialOption{grpc.WithInsecure()}
	dialOpts = append(dialOpts,
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout(protocol, addr, timeout)
		},
		))

	conn, err := grpc.Dial(addr, dialOpts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

type transport struct {
	cluster *Cluster
}

// Join a new machine in the cluster
func (t *transport) Join(ctx context.Context, in *NodeInfo) (*JoinResponse, error) {
	go t.registerNode(in)

	return &JoinResponse{}, nil
}

// Receive message from raft backend
func (t *transport) Receive(ctx context.Context, in *Message) (*Acknowledgment, error) {
	// TODO receive logic

	return &Acknowledgment{}, nil
}

// Handle Node join event
func (t *transport) registerNode(node *NodeInfo) {
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

	t.cluster.AddNodes(
		&Node{
			Client: client,
			Status: status,
			Error:  err,
		},
	)
}

// Listen initiates the gossip mechanism
func Register(server *grpc.Server, cluster *Cluster) {
	RegisterProtonServer(server, &transport{cluster: cluster})
}
