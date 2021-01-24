# Benzene

Benzene is a toy blockchain sharding system based on go-ethereum and Harmony.

## Requirements

On Linux (Ubuntu)

```shell
sudo apt install glibc-static gmp-devel gmp-static openssl-libs openssl-static gcc-c++
```

## Installment

1. Create the appropriate directories:
```shell
mkdir -p $(go env GOPATH)/src/github.com/harmony-one
cd $(go env GOPATH)/src/github.com/harmony-one
```

If you get 'unknown command' or something along those lines, make sure to install golang first.

2. Clone this repo & dependent repos.
```shell
git clone https://github.com/harmony-one/mcl.git
git clone https://github.com/harmony-one/bls.git
git clone https://github.com/hongzicong/benzene.git
cd $(go env GOPATH)/src/github.com/hongzicong/benzene
```

3. Run bash `scripts/install_build_tools.sh` to ensure build tools are of correct versions.

4. Build the harmony binary & dependent libs
```shell
make
```

## Design Overview

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