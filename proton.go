package proton

import (
	"errors"
	"net"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
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
