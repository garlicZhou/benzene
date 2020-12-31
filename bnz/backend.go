package bnz

import (
	"benzene/api/proto"
	msg_pb "benzene/api/proto/message"
	proto_node "benzene/api/proto/node"
	"benzene/api/service"
	"benzene/consensus"
	consensus_engine "benzene/consensus/engine"
	"benzene/core"
	"benzene/core/types"
	"benzene/internal/bnzapi"
	"benzene/internal/chain"
	"benzene/internal/configs"
	"benzene/node"
	"benzene/p2p"
	"benzene/params"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	protobuf "github.com/golang/protobuf/proto"
	"github.com/harmony-one/harmony/crypto/bls"
	libp2p_peer "github.com/libp2p/go-libp2p-core/peer"
	libp2p_pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"
	"sync"
	"time"
)

const (
	// NumTryBroadCast is the number of times trying to broadcast
	NumTryBroadCast = 3
	// MsgChanBuffer is the buffer of consensus message handlers.
	MsgChanBuffer = 1024
)

type withError struct {
	err     error
	payload interface{}
}

var (
	errMsgHadNoHMYPayLoadAssumption      = errors.New("did not have sufficient size for bnz msg")
	errConsensusMessageOnUnexpectedTopic = errors.New("received consensus on wrong topic")
)

var (
	errNotRightKeySize   = errors.New("key received over wire is wrong size")
	errNoSenderPubKey    = errors.New("no sender public BLS key in message")
	errWrongSizeOfBitmap = errors.New("wrong size of sender bitmap")
	errWrongShardID      = errors.New("wrong shard id")
	errInvalidNodeMsg    = errors.New("invalid node message")
	errIgnoreBeaconMsg   = errors.New("ignore beacon sync block")
	errInvalidEpoch      = errors.New("invalid epoch for transaction")
	errInvalidShard      = errors.New("invalid shard")
)

// Benzene implements the Benzene full node service.
type Benzene struct {
	config *Config

	// Handlers
	txPool map[uint64]*core.TxPool
	// TODO: use Protocol design in go-ethereum (hongzicong)
	//protocolManager *ProtocolManager

	// DB interfaces
	chainDbs map[uint64]ethdb.Database // Blockchain database

	eventMux  *event.TypeMux
	Consensus *consensus.Consensus // Consensus object containing all Consensus related data (e.g. committee members, signatures, commits)
	engine    consensus_engine.Engine

	shardChains core.Collection // Shard databases

	APIBackend *BnzAPIBackend

	selfHost p2p.Host
	SelfPeer p2p.Peer

	// TODO: Neighbors should store only neighbor nodes in the same shard
	Neighbors sync.Map // All the neighbor nodes, key is the sha256 of Peer IP/Port, value is the p2p.Peer

	// Channel to notify consensus service to really start consensus
	startConsensus chan struct{}
}

// New creates a new Benzene object (including the
// initialisation of the common Benzene object)
func New(stack *node.Node, config *Config) (*Benzene, error) {
	if config.NoPruning && config.TrieDirtyCache > 0 {
		if config.SnapshotCache > 0 {
			config.TrieCleanCache += config.TrieDirtyCache * 3 / 5
			config.SnapshotCache += config.TrieDirtyCache * 2 / 5
		} else {
			config.TrieCleanCache += config.TrieDirtyCache
		}
		config.TrieDirtyCache = 0
	}
	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	bnz := &Benzene{
		config:         config,
		txPool:         make(map[uint64]*core.TxPool),
		chainDbs:       make(map[uint64]ethdb.Database),
		eventMux:       stack.EventMux(),
		selfHost:       stack.SelfHost,
		startConsensus: make(chan struct{}),
	}

	consensusObj, err := consensus.New(
		bnz.selfHost, config.ShardID, config.ConsensusPriKey,
	)
	if err != nil {
		return nil, err
	}
	bnz.Consensus = consensusObj

	var chainConfig *params.ChainConfig
	for _, shardid := range config.ShardID {
		bnz.chainDbs[shardid], err = stack.OpenDatabase(shardid, "chaindata", config.DatabaseCache, config.DatabaseHandles, "bnz/db/chaindata/")
		if err != nil {
			return nil, err
		}
		chainConfig, _, genesisErr := core.SetupGenesisBlock(bnz.chainDbs[shardid], config.Genesis[shardid])
		if genesisErr != nil {
			return nil, genesisErr
		}
		bnz.engine = CreateConsensusEngine(stack, chainConfig, bnz.chainDbs)
		log.Info("Initialised chain configuration", "config", chainConfig)
	}

	var (
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanJournal:    stack.ResolvePath(config.TrieCleanCacheJournal),
			TrieCleanRejournal:  config.TrieCleanCacheRejournal,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
			SnapshotLimit:       config.SnapshotCache,
		}
	)

	collection := core.NewCollection(bnz.chainDbs, bnz.engine, cacheConfig, chainConfig, bnz.shouldPreserve, &config.TxLookupLimit)
	bnz.shardChains = collection

	for _, shardid := range config.ShardID {
		if config.TxPool.Journal != "" {
			config.TxPool.Journal = stack.ResolvePath(config.TxPool.Journal)
		}
		bnz.txPool[shardid] = core.NewTxPool(config.TxPool, chainConfig, bnz.Blockchain(shardid))
	}
	bnz.APIBackend = &BnzAPIBackend{stack.Config().ExtRPCEnabled(), bnz}

	// Register the backend on the node
	stack.RegisterAPIs(bnz.APIs())
	stack.RegisterLifecycle(bnz)
	return bnz, nil
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Ethereum service
func CreateConsensusEngine(stack *node.Node, chainConfig *params.ChainConfig, dbs map[uint64]ethdb.Database) consensus_engine.Engine {
	// TODO: return our consensus engine (hongzicong)
	return &chain.EngineImpl{}
}

func (bnz *Benzene) initNodeConfiguration() (service.NodeConfig, chan p2p.Peer, error) {
	chanPeer := make(chan p2p.Peer)
	nodeConfig := service.NodeConfig{
		ShardGroupID: bnz.config.GroupID,
		Actions:      map[configs.GroupID]configs.ActionType{},
	}

	// force the side effect of topic join
	if err := bnz.selfHost.SendMessageToGroups(bnz.config.GroupID, []byte{}); err != nil {
		return nodeConfig, nil, err
	}

	return nodeConfig, chanPeer, nil
}

// APIs return the collection of RPC services the benzene package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (bnz *Benzene) APIs() []rpc.API {
	apis := bnzapi.GetAPIs(bnz.APIBackend)

	// TODO: provide more apis (hongzicong)
	return append(apis, []rpc.API{}...)
}

// isLocalBlock checks whether the specified block is mined
// by local miner accounts.
func (bnz *Benzene) isLocalBlock(block *types.Block) bool {
	// TODO: to verify whether a block is proposed locally (hongzicong)
	return false
}

// shouldPreserve checks whether we should preserve the given block
// during the chain reorg depending on whether the author of block
// is a local account.
func (bnz *Benzene) shouldPreserve(block *types.Block) bool {
	return bnz.isLocalBlock(block)
}

// Blockchain returns the blockchain for the node's current shard.
func (bnz *Benzene) Blockchain(shardid uint64) *core.BlockChain {
	bc, err := bnz.shardChains.ShardChain(shardid)
	if err != nil {
		log.Error("cannot get shard chain", "shardID", shardid, "err", err)
	}
	return bc
}

func (bnz *Benzene) TxPool(shardid uint64) *core.TxPool    { return bnz.txPool[shardid] }
func (bnz *Benzene) EventMux() *event.TypeMux              { return bnz.eventMux }
func (bnz *Benzene) Engine() consensus_engine.Engine       { return bnz.engine }
func (bnz *Benzene) ChainDb(shardid uint64) ethdb.Database { return bnz.chainDbs[shardid] }

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Benzene protocol implementation.
func (bnz *Benzene) Start() error {
	return nil
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Benzene protocol.
func (bnz *Benzene) Stop() error {
	// Then stop everything else.
	for _, shardid := range bnz.config.ShardID {
		bnz.txPool[shardid].Stop()
	}
	bnz.shardChains.Close()
	bnz.eventMux.Stop()
	return nil
}

// AddPendingTransaction adds one new transaction to the pending transaction list.
// This is only called from SDK.
func (bnz *Benzene) AddPendingTransaction(newTx *types.Transaction) error {
	for _, shardid := range bnz.config.ShardID {
		if newTx.ShardID() == shardid {
			errs := bnz.addPendingTransactions(types.Transactions{newTx})
			var err error
			for i := range errs {
				if errs[i] != nil {
					log.Info("[AddPendingTransaction] Failed adding new transaction", "err", errs[i])
					err = errs[i]
					break
				}
			}
			if err == nil {
				log.Info("Broadcasting Tx", "Hash", newTx.Hash().Hex())
				bnz.tryBroadcast(newTx)
			}
			return err
		}
	}
	return errors.New("shard do not match")
}

// Add new transactions to the pending transaction list.
func (bnz *Benzene) addPendingTransactions(newTxs types.Transactions) []error {
	poolTxs := make(map[uint64]types.Transactions)
	var errs []error
	for _, tx := range newTxs {
		// TODO: change this validation rule according to the cross-shard mechanism (hongzicong)
		if tx.ShardID() != tx.ToShardID() {
			errs = append(errs, errors.New("cross-shard tx not accepted yet"))
			continue
		}
		poolTxs[tx.ShardID()] = append(poolTxs[tx.ShardID()], tx)
	}
	for _, shardid := range bnz.config.ShardID {
		errs = append(errs, bnz.TxPool(shardid).AddLocals(poolTxs[shardid])...)
		pendingCount, queueCount := bnz.TxPool(shardid).Stats()
		log.Info("[addPendingTransactions] Adding more transactions",
			"err", errs,
			"length of newTxs", len(newTxs),
			"totalPending", pendingCount,
			"totalQueued", queueCount)
	}
	return errs
}

// TODO: make this batch more transactions
// Broadcast the transaction to nodes with the topic shardGroupID
func (bnz *Benzene) tryBroadcast(tx *types.Transaction) {
	msg := proto_node.ConstructTransactionListMessageAccount(types.Transactions{tx})

	shardGroupID := configs.NewGroupIDByShardID(tx.ShardID())
	log.Info("tryBroadcast", "shardGroupID", string(shardGroupID))

	for attempt := 0; attempt < NumTryBroadCast; attempt++ {
		if err := bnz.selfHost.SendMessageToGroups([]configs.GroupID{shardGroupID}, p2p.ConstructMessage(msg)); err != nil {
			log.Error("Error when trying to broadcast tx", "attempt", attempt)
		} else {
			break
		}
	}
}

// AddPeers adds neighbors nodes
func (bnz *Benzene) AddPeers(peers []*p2p.Peer) int {
	for _, p := range peers {
		key := fmt.Sprintf("%s:%s:%s", p.IP, p.Port, p.PeerID)
		_, ok := bnz.Neighbors.LoadOrStore(key, *p)
		if !ok {
			// !ok means new peer is stored
			bnz.selfHost.AddPeer(p)
			continue
		}
	}

	return bnz.selfHost.GetPeerCount()
}

func (bnz *Benzene) startMessageHandle() error {
	// groupID and whether this topic is used for consensus
	groups := map[configs.GroupID]bool{}
	for _, tp := range bnz.config.GroupID {
		groups[tp] = true
	}
	for _, tp := range bnz.config.ClientID {
		if _, ok := groups[tp]; !ok {
			groups[tp] = false
		}
	}

	log.Debug("starting with these topics", "topics-ended-up-with", groups, "shard-id", bnz.config.ShardID)

	type u struct {
		p2p.NamedTopic
		consensusBound bool
	}
	var allTopics []u

	for key, isCon := range groups {
		topicHandle, err := bnz.selfHost.GetOrJoin(string(key))
		if err != nil {
			return err
		}
		allTopics = append(
			allTopics, u{
				NamedTopic:     p2p.NamedTopic{Name: string(key), Topic: topicHandle},
				consensusBound: isCon,
			},
		)
	}
	pubsub := bnz.selfHost.PubSub()
	ownID := bnz.selfHost.GetID()
	errChan := make(chan withError, 100)

	// p2p consensus message handler function
	type p2pHandlerConsensus func(
		ctx context.Context,
		msg *msg_pb.Message,
		key *bls.SerializedPublicKey,
	) error

	// other p2p message handler function
	type p2pHandlerElse func(
		ctx context.Context,
		rlpPayload []byte,
		actionType proto_node.MessageType,
	) error

	// interface pass to p2p message validator
	type validated struct {
		consensusBound bool
		handleC        p2pHandlerConsensus
		handleCArg     *msg_pb.Message
		handleE        p2pHandlerElse
		handleEArg     []byte
		senderPubKey   *bls.SerializedPublicKey
		actionType     proto_node.MessageType
	}

	for i := range allTopics {
		sub, err := allTopics[i].Topic.Subscribe()
		if err != nil {
			return err
		}

		topicNamed := allTopics[i].Name
		isConsensusBound := allTopics[i].consensusBound

		log.Info("enabled topic validation pubsub messages", "topic", topicNamed)

		// register topic validator for each topic
		if err := pubsub.RegisterTopicValidator(
			topicNamed,
			// this is the validation function called to quickly validate every p2p message
			// there will be two validation results, i.e., libp2p_pubsub.ValidationReject and libp2p_pubsub.ValidationAccept
			func(ctx context.Context, peer libp2p_peer.ID, msg *libp2p_pubsub.Message) libp2p_pubsub.ValidationResult {
				bnzMsg := msg.GetData()

				// validate the size of the p2p message
				if len(bnzMsg) < p2pMsgPrefixSize {
					// TODO (lc): block peers sending empty messages
					return libp2p_pubsub.ValidationReject
				}

				openBox := bnzMsg[p2pMsgPrefixSize:]

				// validate message category
				switch proto.MessageCategory(openBox[proto.MessageCategoryBytes-1]) {
				case proto.Consensus:
					// received consensus message in non-consensus bound topic
					if !isConsensusBound {
						errChan <- withError{
							errors.WithStack(errConsensusMessageOnUnexpectedTopic), msg,
						}
						return libp2p_pubsub.ValidationReject
					}

					// validate consensus message
					validMsg, senderPubKey, ignore, err := bnz.validateShardBoundMessage(
						context.TODO(), openBox[proto.MessageCategoryBytes:],
					)
					if err != nil {
						errChan <- withError{err, msg.GetFrom()}
						return libp2p_pubsub.ValidationReject
					}

					// ignore the further processing of the p2p messages as it is not intended for this node
					if ignore {
						return libp2p_pubsub.ValidationAccept
					}

					// if not ignore, continue the further processing
					msg.ValidatorData = validated{
						consensusBound: true,
						handleC:        bnz.Consensus.HandleConsensusMessage,
						handleCArg:     validMsg,
						senderPubKey:   senderPubKey,
					}
					return libp2p_pubsub.ValidationAccept

				case proto.Node:
					// node message is almost empty
					if len(openBox) <= p2pNodeMsgPrefixSize {
						return libp2p_pubsub.ValidationReject
					}
					validMsg, actionType, err := bnz.validateNodeMessage(
						context.TODO(), openBox,
					)
					if err != nil {
						switch err {
						//case errIgnoreBeaconMsg:
						//	// ignore the further processing of the ignored messages as it is not intended for this node
						//	// but propogate the messages to other nodes
						//	return libp2p_pubsub.ValidationAccept
						default:
							// TODO (lc): block peers sending error messages
							errChan <- withError{err, msg.GetFrom()}
							return libp2p_pubsub.ValidationReject
						}
					}
					msg.ValidatorData = validated{
						consensusBound: false,
						handleE:        bnz.HandleNodeMessage,
						handleEArg:     validMsg,
						actionType:     actionType,
					}
					return libp2p_pubsub.ValidationAccept
				default:
					// ignore garbled messages
					return libp2p_pubsub.ValidationReject
				}
			},
			// WithValidatorTimeout is an option that sets a timeout for an (asynchronous) topic validator. By default there is no timeout in asynchronous validators.
			libp2p_pubsub.WithValidatorTimeout(250*time.Millisecond),
			// WithValidatorConcurrency set the concurernt validator, default is 1024
			libp2p_pubsub.WithValidatorConcurrency(p2p.SetAsideForConsensus),
			// WithValidatorInline is an option that sets the validation disposition to synchronous:
			// it will be executed inline in validation front-end, without spawning a new goroutine.
			// This is suitable for simple or cpu-bound validators that do not block.
			libp2p_pubsub.WithValidatorInline(true),
		); err != nil {
			return err
		}

		semConsensus := semaphore.NewWeighted(p2p.SetAsideForConsensus)
		msgChanConsensus := make(chan validated, MsgChanBuffer)

		// goroutine to handle consensus messages
		go func() {
			for m := range msgChanConsensus {
				// should not take more than 10 seconds to process one message
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				msg := m
				go func() {
					defer cancel()

					if semConsensus.TryAcquire(1) {
						defer semConsensus.Release(1)
						if err := msg.handleC(ctx, msg.handleCArg, msg.senderPubKey); err != nil {
							errChan <- withError{err, msg.senderPubKey}
						}
					}

					select {
					case <-ctx.Done():
						if errors.Is(ctx.Err(), context.DeadlineExceeded) {
							log.Warn("[context] exceeded consensus message handler deadline", "topic", topicNamed)
						}
						errChan <- withError{errors.WithStack(ctx.Err()), nil}
					default:
						return
					}
				}()
			}
		}()

		semNode := semaphore.NewWeighted(p2p.SetAsideOtherwise)
		msgChanNode := make(chan validated, MsgChanBuffer)

		// goroutine to handle node messages
		go func() {
			for m := range msgChanNode {
				// should not take more than 10 seconds to process one message
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				msg := m
				go func() {
					defer cancel()
					if semNode.TryAcquire(1) {
						defer semNode.Release(1)

						if err := msg.handleE(ctx, msg.handleEArg, msg.actionType); err != nil {
							errChan <- withError{err, nil}
						}
					}

					select {
					case <-ctx.Done():
						if errors.Is(ctx.Err(), context.DeadlineExceeded) {
							log.Warn("[context] exceeded node message handler deadline", "topic", topicNamed)
						}
						errChan <- withError{errors.WithStack(ctx.Err()), nil}
					default:
						return
					}
				}()
			}
		}()

		// goroutine to retrieve message from the subscribed topic and
		// distribute validated messages to msgChanConsensus and msgChanNode
		go func() {
			for {
				nextMsg, err := sub.Next(context.Background())
				if err != nil {
					errChan <- withError{errors.WithStack(err), nil}
					continue
				}

				if nextMsg.GetFrom() == ownID {
					continue
				}

				if validatedMessage, ok := nextMsg.ValidatorData.(validated); ok {
					if validatedMessage.consensusBound {
						msgChanConsensus <- validatedMessage
					} else {
						msgChanNode <- validatedMessage
					}
				} else {
					// continue if ValidatorData is nil
					if nextMsg.ValidatorData == nil {
						continue
					}
				}
			}
		}()
	}

	for e := range errChan {
		log.Info("[p2p]: issue while handling incoming p2p message: %v", e.err, "item", e.payload)
	}
	// NOTE never gets here
	return nil
}

// TODO: implement our function to validate consensus message (hongzicong)
// validateShardBoundMessage validate consensus message
// validate shardID
// validate public key size
// verify message signature
func (bnz *Benzene) validateShardBoundMessage(
	ctx context.Context, payload []byte,
) (*msg_pb.Message, *bls.SerializedPublicKey, bool, error) {
	var m msg_pb.Message

	if err := protobuf.Unmarshal(payload, &m); err != nil {
		return nil, nil, true, errors.WithStack(err)
	}

	_, _ = m.GetConsensus(), m.GetViewchange()

	serializedKey := bls.SerializedPublicKey{}

	// serializedKey will be empty for multiSig sender
	return &m, &serializedKey, false, nil
}

// TODO: implement our function to validate node message (hongzicong)
// validateNodeMessage validate node message
func (bnz *Benzene) validateNodeMessage(ctx context.Context, payload []byte) ([]byte, proto_node.MessageType, error) {

	// just ignore payload[0], which is MsgCategoryType (consensus/node)
	msgType := proto_node.MessageType(payload[proto.MessageCategoryBytes])

	switch msgType {
	case proto_node.Transaction:
		// nothing much to validate transaction message unless decode the RLP
	case proto_node.Block:
	default:
		return nil, 0, errInvalidNodeMsg
	}

	return payload[p2pNodeMsgPrefixSize:], msgType, nil
}
