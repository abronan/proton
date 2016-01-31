# proton

Small library for experimenting with etcd's **Raft** backed by **boltdb** and **gRPC**.

## TODO

- Implement Store interface with a boltdb backend
- Add better Node Management and improve Join process
- Monitor connections and gracefully handle node leaving the Raft
- Add simple Compare and Swap Primitives with atomic index increment on key
- Refactor and clean