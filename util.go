package proton

import (
	"errors"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
)

const (
	MaxRetryTime = 3
)

type Raft struct {
	RaftClient
	Conn *grpc.ClientConn
}

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

func getClientConn(addr string, protocol string, timeout time.Duration) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithTimeout(timeout))
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func EncodePair(key string, value []byte) ([]byte, error) {
	k := proto.String(key)
	pair := &Pair{
		Key:   *k,
		Value: value,
	}
	data, err := proto.Marshal(pair)
	if err != nil {
		return nil, errors.New("Can't encode key/value using protobuf")
	}
	return data, nil
}

func Register(server *grpc.Server, node *RaftNode) {
	RegisterRaftServer(server, node)
}
