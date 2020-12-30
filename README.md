# Benzene

## Design Overview

Benzene is a toy blockchain sharding system based on go-ethereum and Harmony.

### Beacon Chain

There is a beacon chain used to record the identities of participants (such as peer id) and their shard ids.

### Service Module

In Harmony, a node runs a certain set of services and use a service manager to manage (start or stop) services.

### Communication Module

The communication modular is supported in benzene/p2p.

Similar to Harmony, instead of sending messages to individual nodes like go-ethereum, Benzene uses libp2p package to gossip message.

All message communication depends on `SendMessageToGroups` function in benzene/p2p.

message ...

protobuf ...

## TODO

1. Resharding
2. New(), closeDatabases, ephemKeystore in node

## Reference

1. https://geth.ethereum.org/docs/