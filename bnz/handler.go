package bnz

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"time"

	"benzene/api/proto"
	proto_node "benzene/api/proto/node"
	"benzene/core/types"
	"benzene/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/pkg/errors"
)

const p2pMsgPrefixSize = 5
const p2pNodeMsgPrefixSize = proto.MessageTypeBytes + proto.MessageCategoryBytes

var (
	errInvalidPayloadSize = errors.New("invalid payload size")
	errWrongBlockMsgSize  = errors.New("invalid block message size")
)

// HandleNodeMessage parses the message and dispatch the actions.
func (bnz *Benzene) HandleNodeMessage(
	ctx context.Context,
	msgPayload []byte,
	actionType proto_node.MessageType,
) error {
	switch actionType {
	case proto_node.Transaction:
		bnz.transactionMessageHandler(msgPayload)
	case proto_node.Block:
		switch blockMsgType := proto_node.BlockMessageType(msgPayload[0]); blockMsgType {
		case proto_node.Sync:
			var blocks []*types.Block
			if err := rlp.DecodeBytes(msgPayload[1:], &blocks); err != nil {
				log.Error("block sync", "error", err)
			} else {
				// TODO: sync block (hongzicong)
			}
		}
	default:
		log.Error("Unknown actionType", string(actionType))
	}
	return nil
}

func (bnz *Benzene) transactionMessageHandler(msgPayload []byte) {
	txMessageType := proto_node.TransactionMessageType(msgPayload[0])

	switch txMessageType {
	case proto_node.Send:
		txs := types.Transactions{}
		err := rlp.Decode(bytes.NewReader(msgPayload[1:]), &txs) // skip the Send messge type
		if err != nil {
			log.Error("Failed to deserialize transaction list", "error", err)
			return
		}
		bnz.addPendingTransactions(txs)
	}
}

// BroadcastNewBlock is called by consensus leader to sync new blocks with other clients/nodes.
// NOTE: For now, just send to the client (basically not broadcasting)
// TODO (lc): broadcast the new blocks to new nodes doing state sync
func (bnz *Benzene) BroadcastNewBlock(newBlock *types.Block) {
	groups := bnz.config.ClientID
	log.Info(fmt.Sprintf("broadcasting new block number %d of shard %d", newBlock.NumberU64(), newBlock.ShardID()), "groups", groups)
	msg := p2p.ConstructMessage(proto_node.ConstructBlocksSyncMessage([]*types.Block{newBlock}))
	if err := bnz.selfHost.SendMessageToGroups(groups, msg); err != nil {
		log.Warn("cannot broadcast new block", "error", err)
	}
}

// VerifyNewBlock is called by consensus participants to verify the block (account model) they are
// running consensus on
func (bnz *Benzene) VerifyNewBlock(newBlock *types.Block) error {
	return nil
}

// PostConsensusProcessing is called by consensus participants, after consensus is done, to:
// 1. add the new block to blockchain
// 2. [leader] send new block to the client
// 3. [leader] send cross shard tx receipts to destination shard
func (bnz *Benzene) PostConsensusProcessing(newBlock *types.Block) error {
	return nil
}

// BootstrapConsensus is the a goroutine to check number of peers and start the consensus
func (bnz *Benzene) BootstrapConsensus() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	min := bnz.Consensus.MinPeers
	enoughMinPeers := make(chan struct{})
	const checkEvery = 3 * time.Second
	go func() {
		for {
			<-time.After(checkEvery)
			numPeersNow := bnz.selfHost.GetPeerCount()
			if numPeersNow >= min {
				log.Info("[bootstrap] StartConsensus")
				enoughMinPeers <- struct{}{}
				return
			}
			log.Info("do not have enough min peers yet in bootstrap of consensus",
				"numPeersNow", numPeersNow,
				"targetNumPeers", min,
				"next-peer-count-check-in-seconds", checkEvery)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-enoughMinPeers:
		go func() {
			bnz.startConsensus <- struct{}{}
		}()
		return nil
	}
}
