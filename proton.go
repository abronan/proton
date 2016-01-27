package proton

import (
	"net"
	"time"

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

func Register(server *grpc.Server, node *Node) {
	RegisterProtonServer(server, node)
}
