# Benzene

## Design Overview

Benzene is a toy blockchain sharding system based on go-ethereum and Harmony.

### Service Module

In Harmony, a node runs a certain set of services and use a service manager to manage (start or stop) services.

### Communication Module

The communication modular is supported in benzene/p2p.

Similar to Harmony, instead of sending messages to individual nodes like go-ethereum, Benzene uses libp2p package to gossip message.

All message communication depends on `SendMessageToGroups` function in benzene/p2p.

## Reference

1. https://geth.ethereum.org/docs/