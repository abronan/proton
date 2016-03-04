package proton

import (
	"errors"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
)

const (
	// MaxRetryTime is the number of time we try to initiate
	// a grpc connection to a remote raft member
	MaxRetryTime = 3
)

// Raft represents a connection to a raft member
type Raft struct {
	RaftClient
	Conn *grpc.ClientConn
}

// GetRaftClient returns a raft client object to communicate
// with other raft members
func GetRaftClient(addr string, timeout time.Duration) (*Raft, error) {
	conn, err := getClientConn(addr, "tcp", timeout)
	if err != nil {
		return nil, err
	}

	return &Raft{
		RaftClient: NewRaftClient(conn),
		Conn:       conn,
	}, nil
}

// getClientConn returns a grpc client connection
func getClientConn(addr string, protocol string, timeout time.Duration) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithTimeout(timeout))
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// EncodePair returns a protobuf encoded key/value pair to be sent through raft
func EncodePair(key string, value []byte) ([]byte, error) {
	pair := &Proposal_Pair{
		&Pair{Key: key, Value: value},
	}
	proposal := &Proposal{
		Proposal: pair,
	}
	data, err := proto.Marshal(proposal)
	if err != nil {
		return nil, errors.New("Can't encode key/value using protobuf")
	}
	return data, nil
}

// EncodeDiff returns a protobuf encoded set of key/value
// pairs to be sent through raft
func EncodeDiff(objects []*Pair) ([]byte, error) {
	diff := &Proposal_Diff{
		&Diff{objects},
	}
	proposal := &Proposal{
		Proposal: diff,
	}
	data, err := proto.Marshal(proposal)
	if err != nil {
		return nil, errors.New("Can't encode key/value using protobuf")
	}
	return data, nil
}

// Register registers the node raft server
func Register(server *grpc.Server, node *Node) {
	RegisterRaftServer(server, node)
}
