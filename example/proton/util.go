package main

import (
	"fmt"
	"log"

	"github.com/abronan/proton"
	"github.com/gogo/protobuf/proto"
)

func handler(msg interface{}) {
	// TODO two types: Object and ObjectSet (for merging diffs)

	// Here: can be a protobuf 'oneof' message
	pair := &proton.Pair{}
	err := proto.Unmarshal(msg.([]byte), pair)
	if err != nil {
		log.Fatal("Can't decode key and value sent through raft")
	}
	fmt.Printf("New entry added to logs: [%v = %v]\n", pair.Key, string(pair.Value))
}
