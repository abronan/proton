# proton CLI

Example cli application running etcd's **Raft** backed by **boltdb** and **gRPC**.

The **init** process assumes it's the leader and creates the Raft while other processes are joining the Raft and *replicating logs* written by the **init** node.

## Example

#### Init
```
# proton init -H 0.0.0.0:5000 --hostname "Bob"
```

#### Join
```
# proton join -H 0.0.0.0:6000 --join 0.0.0.0:5000 --hostname "Sarah"
# proton join -H 0.0.0.0:7000 --join 0.0.0.0:5000 --hostname "Clark"
```

#### Enable Raft debug mode
```
# proton init --withRaftLogs -H 0.0.0.0:5000 --hostname "Bob"
```
