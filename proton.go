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

type Proton struct {
	Client ProtonClient
	Conn   *grpc.ClientConn
}

func GetProtonClient(addr string, timeout time.Duration) (*Proton, error) {
	conn, err := getClientConn(addr, "tcp", timeout)
	if err != nil {
		return nil, err
	}

	return &Proton{
		Client: NewProtonClient(conn),
		Conn:   conn,
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

func Register(server *grpc.Server, node *Node) {
	RegisterProtonServer(server, node)
}
