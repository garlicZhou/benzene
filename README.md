# Benzene

Benzene is a toy blockchain sharding system based on go-ethereum (v1.9.24) and Harmony.

## Requirements

On Linux (Ubuntu 18.04)

Go 1.14.1

```shell
sudo apt install libgmp-dev libssl-dev curl git \
psmisc dnsutils jq make gcc g++ bash tig tree sudo vim \
silversearcher-ag unzip emacs-nox nano bash-completion -y
```

## Installment

1. Clone this repo & dependent repos.
```shell
mkdir -p $(go env GOPATH)/src/github.com/harmony-one
cd $(go env GOPATH)/src/github.com/harmony-one
git clone https://github.com/harmony-one/mcl.git
git clone https://github.com/harmony-one/bls.git
```

```shell
mkdir -p $(go env GOPATH)/src/github.com/hongzicong
cd $(go env GOPATH)/src/github.com/hongzicong
git clone https://github.com/hongzicong/benzene.git
cd $(go env GOPATH)/src/github.com/hongzicong/benzene
```

If you get 'unknown command' or something along those lines, make sure to install golang first.

3. Set `chmod 777 scripts/*.sh` Run bash `scripts/install_build_tools.sh` to ensure build tools are of correct versions.

4. Build the binary & dependent libs
```shell
make
```

5. You can find the binary and dependent libs in build/bin.

```shell
./build/bin/benzene --datadir=./bin --p2p.keyfile=./bin/.bnzkey -run.shard=1 --http --http.api personal,bnz --allow-insecure-unlock
```

6. If you get an error `error while loading shared libraries: libbls384_256.so: cannot open shared object file: No such file or directory`, you need to define the following environment variable.

```shell
export CGO_CFLAGS="-I$(go env GOPATH)/src/github.com/harmony-one/bls/include -I$(go env GOPATH)/src/github.com/harmony-one/mcl/include -I/usr/local/opt/openssl/include"
export CGO_LDFLAGS="-L$(go env GOPATH)/src/github.com/harmony-one/bls/lib -L/usr/local/opt/openssl/lib"
export LD_LIBRARY_PATH="$(go env GOPATH)/src/github.com/harmony-one/bls/lib:$(go env GOPATH)/src/github.com/harmony-one/mcl/lib:/usr/local/opt/openssl/lib"
export LIBRARY_PATH=$LD_LIBRARY_PATH
export DYLD_FALLBACK_LIBRARY_PATH=$LD_LIBRARY_PATH
export GO111MODULE=on
```

## Design Overview

### Beacon Chain

There is a beacon chain used to record the identities of participants (such as peer id) and their shard ids.

### Service Module

Similar to Harmony, a node runs a certain set of services and use a service manager to manage (start or stop) services.

### Communication Module

The communication modular is supported in benzene/p2p.

Similar to Harmony, instead of sending messages to individual nodes like go-ethereum, Benzene uses libp2p package to gossip message.

All message communication depends on `SendMessageToGroups` function in benzene/p2p.

## RPC API

1. GetShardID

Returns the shard id of the node

```shell
curl -d '{
    "jsonrpc":"2.0",
    "method":"bnz_getShardID",
    "params":[],
    "id":1
}' -H "Content-Type:application/json" -X POST "localhost:8545"
```

2. BlockNumber

Returns the block number of a shard

```shell
curl -d '{
    "jsonrpc":"2.0",
    "method":"bnz_blockNumber",
    "params":[1],
    "id":1
}' -H "Content-Type:application/json" -X POST "localhost:8545"
```

3. GetBalance

```shell
curl -d '{
    "jsonrpc":"2.0",
	"method":"bnz_getBalance",
	"params":[1, "0xc2d7cf95645d33006175b78989035c7c9061d3f9", "latest"],
	"id":1
}' -H "Content-Type:application/json" -X POST "localhost:8545"
```

ps. In `core/genesis_alloc.go`, we have pre-allocated 300000 to 0x7df9a875a174b3bc565e6424a0050ebc1b2d1d82.

If you want to pre-allocate money to accounts, you can add accounts and the corresponding money to `core/prealloc_account.json`, run `mkalloc.go` and copy the output to `core/genesis_alloc.go`.

4. ListWallets

Returns all the account addresses of all keys in the key store.

```shell
curl -d '{
    "jsonrpc":"2.0",
	"method":"personal_listWallets",
	"params":[],
	"id":1
}' -H "Content-Type:application/json" -X POST "localhost:8545"
```

5. UnlockAccount

Decrypts the key with the given address from the key store.

```shell
curl -d '{
    "jsonrpc":"2.0",
	"method":"personal_unlockAccount",
	"params":["0xc2d7cf95645d33006175b78989035c7c9061d3f9","123456"],
	"id":1
}' -H "Content-Type:application/json" -X POST "localhost:8545"
```

6. SendTransaction 

```shell
curl -d '{
    "jsonrpc":"2.0",
    "method":"bnz_sendTransaction",
    "params":[{
        "from": "0xc2d7cf95645d33006175b78989035c7c9061d3f9",
        "to": "0xd46e8dd67c5d32be8058bb8eb970870f07244567",
        "shardID": "0x1",
        "toShardID": "0x1",
        "gas": "0x76c0",
        "gasPrice": "0x76c0",
        "value": "0x76c0"
    }],
    "id":1
}' -H "Content-Type:application/json" -X POST "localhost:8545"
```

## TODO

1. Intra-shard collective signing-based consensus (Feb. 10, ZHOU Enyuan)

2. Cross-shard transaction processing (Feb. 6, HONG Zicong)

3. Large-scale deployment (Feb. 6, HONG Zicong)

4. Layer sharding consensus (Feb. 13, HONG Zicong)

5. DRL consensus support (Feb. 20, ZHANG Jianting)

6. Smart contract support (including virtual machines, gas estimation, ...)

## Reference

1. https://geth.ethereum.org/docs/

2. https://github.com/ethereum/go-ethereum

3. https://github.com/harmony-one/harmony