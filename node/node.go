package node

import (
	"benzene/api/proto"
	msg_pb "benzene/api/proto/message"
	proto_node "benzene/api/proto/node"
	"benzene/api/service"
	"benzene/consensus"
	"benzene/core"
	"benzene/core/types"
	"benzene/crypto/bls"
	"benzene/internal/configs"
	"benzene/p2p"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	protobuf "github.com/golang/protobuf/proto"
	libp2p_peer "github.com/libp2p/go-libp2p-core/peer"
	libp2p_pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/fileutil"
	"golang.org/x/sync/semaphore"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

const (
	// NumTryBroadCast is the number of times trying to broadcast
	NumTryBroadCast = 3
	// MsgChanBuffer is the buffer of consensus message handlers.
	MsgChanBuffer = 1024
)

const (
	maxBroadcastNodes       = 10              // broadcast at most maxBroadcastNodes peers that need in sync
	broadcastTimeout  int64 = 60 * 1000000000 // 1 mins
	//SyncIDLength is the length of bytes for syncID
	SyncIDLength = 20
)

type withError struct {
	err     error
	payload interface{}
}

var (
	errMsgHadNoHMYPayLoadAssumption      = errors.New("did not have sufficient size for bnz msg")
	errConsensusMessageOnUnexpectedTopic = errors.New("received consensus on wrong topic")
)

// Node represents a protocol-participating node in the network
type Node struct {
	Consensus *consensus.Consensus // Consensus object containing all Consensus related data (e.g. committee members, signatures, commits)
	SelfPeer  p2p.Peer
	SelfHost  p2p.Host // The p2p host used to send/receive p2p messages

	Neighbors sync.Map // All the neighbor nodes, key is the sha256 of Peer IP/Port, value is the p2p.Peer

	serviceManager *service.Manager // Service manager.

	eventmux *event.TypeMux
	config   *Config

	log           log.Logger
	dirLock       fileutil.Releaser // prevents concurrent use of instance directory
	stop          chan struct{}     // Channel to wait for termination notifications
	startStopLock sync.Mutex        // Start/Stop are protected by an additional lock
	state         int               // Tracks state of node lifecycle

	lock          sync.Mutex
	lifecycles    []Lifecycle // All registered backends, services, and auxiliary services that have a lifecycle
	rpcAPIs       []rpc.API   // List of APIs currently provided by the node
	http          *httpServer //
	ws            *httpServer //
	ipc           *ipcServer  // Stores information about the ipc http server
	inprocHandler *rpc.Server // In-process RPC request handler to process the API requests

	databases map[*closeTrackingDB]struct{} // All open databases
}

const (
	initializingState = iota
	runningState
	closedState
)

// New creates a new P2P node, ready for protocol registration.
func New(
	host p2p.Host,
	consensusObj *consensus.Consensus,
	conf *Config,
) (*Node, error) {
	// Copy config and resolve the datadir so future changes to the current
	// working directory don't affect the node.
	confCopy := *conf
	conf = &confCopy
	if conf.DataDir != "" {
		absdatadir, err := filepath.Abs(conf.DataDir)
		if err != nil {
			return nil, err
		}
		conf.DataDir = absdatadir
	}
	if conf.Logger == nil {
		conf.Logger = log.New()
	}
	// Ensure that the instance name doesn't cause weird conflicts with
	// other files in the data directory.
	if strings.ContainsAny(conf.Name, `/\`) {
		return nil, errors.New(`Config.Name must not contain '/' or '\'`)
	}
	if conf.Name == datadirDefaultKeyStore {
		return nil, errors.New(`Config.Name cannot be "` + datadirDefaultKeyStore + `"`)
	}
	if strings.HasSuffix(conf.Name, ".ipc") {
		return nil, errors.New(`Config.Name cannot end in ".ipc"`)
	}

	node := Node{
		config:        conf,
		inprocHandler: rpc.NewServer(),
		eventmux:      new(event.TypeMux),
		log:           conf.Logger,
		stop:          make(chan struct{}),
		databases:     make(map[*closeTrackingDB]struct{}),
	}

	// Register built-in APIs.
	node.rpcAPIs = append(node.rpcAPIs, node.apis()...)

	if host != nil {
		node.SelfHost = host
		node.SelfPeer = host.GetSelfPeer()
	}

	if host != nil && consensusObj != nil {
		node.Consensus = consensusObj
	}

	// Configure RPC servers.
	node.http = newHTTPServer(node.log, conf.HTTPTimeouts)
	node.ws = newHTTPServer(node.log, rpc.DefaultHTTPTimeouts)
	node.ipc = newIPCServer(node.log, conf.IPCEndpoint())

	return &node, nil
}

// Start kicks off the node message handling
func (n *Node) Start() error {
	n.startStopLock.Lock()
	defer n.startStopLock.Unlock()

	n.lock.Lock()
	switch n.state {
	case runningState:
		n.lock.Unlock()
		return ErrNodeRunning
	case closedState:
		n.lock.Unlock()
		return ErrNodeStopped
	}
	n.state = runningState
	err := n.startNetworking()
	lifecycles := make([]Lifecycle, len(n.lifecycles))
	copy(lifecycles, n.lifecycles)
	n.lock.Unlock()

	// Check if networking startup failed.
	if err != nil {
		n.doClose(nil)
		return err
	}

	// TODO: check whether the above lock is ok
	err = n.startMessageHandle()
	if err != nil {
		n.doClose(nil)
		return err
	}

	// Start all registered lifecycles.
	var started []Lifecycle
	for _, lifecycle := range lifecycles {
		if err = lifecycle.Start(); err != nil {
			break
		}
		started = append(started, lifecycle)
	}
	// Check if any lifecycle failed to start.
	if err != nil {
		n.stopServices(started)
		n.doClose(nil)
	}
	return err
}

func (n *Node) startMessageHandle() error {
	// groupID and whether this topic is used for consensus
	groups := map[configs.GroupID]bool{}
	for _, tp := range n.config.GroupID {
		groups[tp] = true
	}
	for _, tp := range n.config.ClientID {
		if _, ok := groups[tp]; !ok {
			groups[tp] = false
		}
	}

	log.Debug("starting with these topics", "topics-ended-up-with", groups, "shard-id", n.config.ShardID)

	type u struct {
		p2p.NamedTopic
		consensusBound bool
	}
	var allTopics []u

	for key, isCon := range groups {
		topicHandle, err := n.SelfHost.GetOrJoin(string(key))
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
	pubsub := n.SelfHost.PubSub()
	ownID := n.SelfHost.GetID()
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
					validMsg, senderPubKey, ignore, err := n.validateShardBoundMessage(
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

					msg.ValidatorData = validated{
						consensusBound: true,
						handleC:        n.Consensus.HandleMessageUpdate,
						handleCArg:     validMsg,
						senderPubKey:   senderPubKey,
					}
					return libp2p_pubsub.ValidationAccept

				case proto.Node:
					// node message is almost empty
					if len(openBox) <= p2pNodeMsgPrefixSize {
						return libp2p_pubsub.ValidationReject
					}
					validMsg, actionType, err := n.validateNodeMessage(
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
						handleE:        n.HandleNodeMessage,
						handleEArg:     validMsg,
						actionType:     actionType,
					}
					return libp2p_pubsub.ValidationAccept
				default:
					// ignore garbled messages
					return libp2p_pubsub.ValidationReject
				}

				select {
				case <-ctx.Done():
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						log.Warn("[context] exceeded validation deadline", "topic", topicNamed)
					}
					errChan <- withError{errors.WithStack(ctx.Err()), nil}
				default:
					return libp2p_pubsub.ValidationAccept
				}

				return libp2p_pubsub.ValidationReject
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

// validateNodeMessage validate node message
func (node *Node) validateNodeMessage(ctx context.Context, payload []byte) (
	[]byte, proto_node.MessageType, error) {

	// length of payload must > p2pNodeMsgPrefixSize

	// reject huge node messages
	if len(payload) >= types.MaxEncodedPoolTransactionSize {
		nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "invalid_oversized"}).Inc()
		return nil, 0, core.ErrOversizedData
	}

	// just ignore payload[0], which is MsgCategoryType (consensus/node)
	msgType := proto_node.MessageType(payload[proto.MessageCategoryBytes])

	switch msgType {
	case proto_node.Transaction:
		// nothing much to validate transaction message unless decode the RLP
		nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "tx"}).Inc()
	case proto_node.Staking:
		// nothing much to validate staking message unless decode the RLP
		nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "staking_tx"}).Inc()
	case proto_node.Block:
		switch proto_node.BlockMessageType(payload[p2pNodeMsgPrefixSize]) {
		case proto_node.Sync:
			nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "block_sync"}).Inc()
			// only non-beacon nodes process the beacon block sync messages
			if node.Blockchain().ShardID() == shard.BeaconChainShardID {
				return nil, 0, errIgnoreBeaconMsg
			}
		case proto_node.SlashCandidate:
			nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "slash"}).Inc()
			// only beacon chain node process slash candidate messages
			if node.NodeConfig.ShardID != shard.BeaconChainShardID {
				return nil, 0, errIgnoreBeaconMsg
			}
		case proto_node.Receipt:
			nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "node_receipt"}).Inc()
		case proto_node.CrossLink:
			nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "crosslink"}).Inc()
			// only beacon chain node process crosslink messages
			if node.NodeConfig.ShardID != shard.BeaconChainShardID ||
				node.NodeConfig.Role() == nodeconfig.ExplorerNode {
				return nil, 0, errIgnoreBeaconMsg
			}
		default:
			nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "invalid_block_type"}).Inc()
			return nil, 0, errInvalidNodeMsg
		}
	default:
		nodeNodeMessageCounterVec.With(prometheus.Labels{"type": "invalid_node_type"}).Inc()
		return nil, 0, errInvalidNodeMsg
	}

	return payload[p2pNodeMsgPrefixSize:], msgType, nil
}

// validateShardBoundMessage validate consensus message
// validate shardID
// validate public key size
// verify message signature
func (n *Node) validateShardBoundMessage(
	ctx context.Context, payload []byte,
) (*msg_pb.Message, *bls.SerializedPublicKey, bool, error) {
	var (
		m msg_pb.Message
	)
	if err := protobuf.Unmarshal(payload, &m); err != nil {
		return nil, nil, true, errors.WithStack(err)
	}

	// ignore messages not intended for explorer
	if n.config.Role() == nodeconfig.ExplorerNode {
		switch m.Type {
		case
			msg_pb.MessageType_ANNOUNCE,
			msg_pb.MessageType_PREPARE,
			msg_pb.MessageType_COMMIT,
			msg_pb.MessageType_VIEWCHANGE,
			msg_pb.MessageType_NEWVIEW:
			return nil, nil, true, nil
		}
	}

	// when node is in ViewChanging mode, it still accepts normal messages into FBFTLog
	// in order to avoid possible trap forever but drop PREPARE and COMMIT
	// which are message types specifically for a node acting as leader
	// so we just ignore those messages
	if n.Consensus.IsViewChangingMode() {
		switch m.Type {
		case msg_pb.MessageType_PREPARE, msg_pb.MessageType_COMMIT:
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "ignored"}).Inc()
			return nil, nil, true, nil
		}
	} else {
		// ignore viewchange/newview message if the node is not in viewchanging mode
		switch m.Type {
		case msg_pb.MessageType_NEWVIEW, msg_pb.MessageType_VIEWCHANGE:
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "ignored"}).Inc()
			return nil, nil, true, nil
		}
	}

	// ignore message not intended for leader, but still forward them to the network
	if n.Consensus.IsLeader() {
		switch m.Type {
		case msg_pb.MessageType_ANNOUNCE, msg_pb.MessageType_PREPARED, msg_pb.MessageType_COMMITTED:
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "ignored"}).Inc()
			return nil, nil, true, nil
		}
	}

	maybeCon, maybeVC := m.GetConsensus(), m.GetViewchange()
	senderKey := []byte{}
	senderBitmap := []byte{}

	if maybeCon != nil {
		if maybeCon.ShardId != n.Consensus.ShardID {
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "invalid_shard"}).Inc()
			return nil, nil, true, errors.WithStack(errWrongShardID)
		}
		senderKey = maybeCon.SenderPubkey

		if len(maybeCon.SenderPubkeyBitmap) > 0 {
			senderBitmap = maybeCon.SenderPubkeyBitmap
		}
	} else if maybeVC != nil {
		if maybeVC.ShardId != n.Consensus.ShardID {
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "invalid_shard"}).Inc()
			return nil, nil, true, errors.WithStack(errWrongShardID)
		}
		senderKey = maybeVC.SenderPubkey
	} else {
		nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "invalid"}).Inc()
		return nil, nil, true, errors.WithStack(errNoSenderPubKey)
	}

	// ignore mesage not intended for validator
	// but still forward them to the network
	if !n.Consensus.IsLeader() {
		switch m.Type {
		case msg_pb.MessageType_PREPARE, msg_pb.MessageType_COMMIT:
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "ignored"}).Inc()
			return nil, nil, true, nil
		}
	}

	serializedKey := bls.SerializedPublicKey{}
	if len(senderKey) > 0 {
		if len(senderKey) != bls.PublicKeySizeInBytes {
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "invalid_key_size"}).Inc()
			return nil, nil, true, errors.WithStack(errNotRightKeySize)
		}

		copy(serializedKey[:], senderKey)
		if !n.Consensus.IsValidatorInCommittee(serializedKey) {
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "invalid_committee"}).Inc()
			return nil, nil, true, errors.WithStack(shard.ErrValidNotInCommittee)
		}
	} else {
		count := n.Consensus.Decider.ParticipantsCount()
		if (count+7)>>3 != int64(len(senderBitmap)) {
			nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "invalid_participant_count"}).Inc()
			return nil, nil, true, errors.WithStack(errWrongSizeOfBitmap)
		}
	}

	nodeConsensusMessageCounterVec.With(prometheus.Labels{"type": "valid"}).Inc()

	// serializedKey will be empty for multiSig sender
	return &m, &serializedKey, false, nil
}

// Close stops the Node and releases resources acquired in
// Node constructor New.
func (n *Node) Close() error {
	n.startStopLock.Lock()
	defer n.startStopLock.Unlock()

	n.lock.Lock()
	state := n.state
	n.lock.Unlock()
	switch state {
	case initializingState:
		// The node was never started.
		return n.doClose(nil)
	case runningState:
		// The node was started, release resources acquired by Start().
		var errs []error
		if err := n.stopServices(n.lifecycles); err != nil {
			errs = append(errs, err)
		}
		return n.doClose(errs)
	case closedState:
		return ErrNodeStopped
	default:
		panic(fmt.Sprintf("node is in unknown state %d", state))
	}
}

// doClose releases resources acquired by New(), collecting errors.
func (n *Node) doClose(errs []error) error {
	// Release instance directory lock.
	n.closeDataDir()

	// Unblock n.Wait.
	close(n.stop)

	// Report any errors that might have occurred.
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return fmt.Errorf("%v", errs)
	}
}

// AddPeers adds neighbors nodes
func (node *Node) AddPeers(peers []*p2p.Peer) int {
	for _, p := range peers {
		key := fmt.Sprintf("%s:%s:%s", p.IP, p.Port, p.PeerID)
		_, ok := node.Neighbors.LoadOrStore(key, *p)
		if !ok {
			// !ok means new peer is stored
			node.SelfHost.AddPeer(p)
			continue
		}
	}

	return node.SelfHost.GetPeerCount()
}

func (node *Node) initNodeConfiguration() (service.NodeConfig, chan p2p.Peer, error) {
	chanPeer := make(chan p2p.Peer)
	nodeConfig := service.NodeConfig{
		ShardGroupID: node.config.GroupID,
		Actions:      map[configs.GroupID]configs.ActionType{},
	}

	// force the side effect of topic join
	if err := node.SelfHost.SendMessageToGroups(node.config.GroupID, []byte{}); err != nil {
		return nodeConfig, nil, err
	}

	return nodeConfig, chanPeer, nil
}

// ServiceManager ...
func (node *Node) ServiceManager() *service.Manager {
	return node.serviceManager
}

// startNetworking starts all network endpoints.
func (n *Node) startNetworking() error {
	//n.log.Info("Starting peer-to-peer node", "instance", n.server.Name)
	//if err := n.server.Start(); err != nil {
	//	return convertFileLockError(err)
	//}
	err := n.startRPC()
	if err != nil {
		n.stopRPC()
		//n.server.Stop()
	}
	return err
}

// containsLifecycle checks if 'lfs' contains 'l'.
func containsLifecycle(lfs []Lifecycle, l Lifecycle) bool {
	for _, obj := range lfs {
		if obj == l {
			return true
		}
	}
	return false
}

// stopServices terminates running services, RPC and p2p networking.
// It is the inverse of Start.
func (n *Node) stopServices(running []Lifecycle) error {
	n.stopRPC()

	// Stop running lifecycles in reverse order.
	failure := &StopError{Services: make(map[reflect.Type]error)}
	for i := len(running) - 1; i >= 0; i-- {
		if err := running[i].Stop(); err != nil {
			failure.Services[reflect.TypeOf(running[i])] = err
		}
	}

	if len(failure.Services) > 0 {
		return failure
	}
	return nil
}

func (n *Node) openDataDir() error {
	if n.config.DataDir == "" {
		return nil // ephemeral
	}

	instdir := filepath.Join(n.config.DataDir, n.config.name())
	if err := os.MkdirAll(instdir, 0700); err != nil {
		return err
	}
	// Lock the instance directory to prevent concurrent use by another instance as well as
	// accidental use of the instance directory as a database.
	release, _, err := fileutil.Flock(filepath.Join(instdir, "LOCK"))
	if err != nil {
		return convertFileLockError(err)
	}
	n.dirLock = release
	return nil
}

func (n *Node) closeDataDir() {
	// Release instance directory lock.
	if n.dirLock != nil {
		if err := n.dirLock.Release(); err != nil {
			n.log.Error("Can't release datadir lock", "err", err)
		}
		n.dirLock = nil
	}
}

// configureRPC is a helper method to configure all the various RPC endpoints during node
// startup. It's not meant to be called at any time afterwards as it makes certain
// assumptions about the state of the node.
func (n *Node) startRPC() error {
	if err := n.startInProc(); err != nil {
		return err
	}

	// Configure IPC.
	if n.ipc.endpoint != "" {
		if err := n.ipc.start(n.rpcAPIs); err != nil {
			return err
		}
	}

	// Configure HTTP.
	if n.config.HTTPHost != "" {
		config := httpConfig{
			CorsAllowedOrigins: n.config.HTTPCors,
			Vhosts:             n.config.HTTPVirtualHosts,
			Modules:            n.config.HTTPModules,
		}
		if err := n.http.setListenAddr(n.config.HTTPHost, n.config.HTTPPort); err != nil {
			return err
		}
		if err := n.http.enableRPC(n.rpcAPIs, config); err != nil {
			return err
		}
	}

	// Configure WebSocket.
	if n.config.WSHost != "" {
		server := n.wsServerForPort(n.config.WSPort)
		config := wsConfig{
			Modules: n.config.WSModules,
			Origins: n.config.WSOrigins,
		}
		if err := server.setListenAddr(n.config.WSHost, n.config.WSPort); err != nil {
			return err
		}
		if err := server.enableWS(n.rpcAPIs, config); err != nil {
			return err
		}
	}

	if err := n.http.start(); err != nil {
		return err
	}
	return n.ws.start()
}

func (n *Node) wsServerForPort(port int) *httpServer {
	if n.config.HTTPHost == "" || n.http.port == port {
		return n.http
	}
	return n.ws
}

func (n *Node) stopRPC() {
	n.http.stop()
	n.ws.stop()
	n.ipc.stop()
	n.stopInProc()
}

// startInProc registers all RPC APIs on the inproc server.
func (n *Node) startInProc() error {
	for _, api := range n.rpcAPIs {
		if err := n.inprocHandler.RegisterName(api.Namespace, api.Service); err != nil {
			return err
		}
	}
	return nil
}

// stopInProc terminates the in-process RPC endpoint.
func (n *Node) stopInProc() {
	n.inprocHandler.Stop()
}

// Wait blocks until the node is closed.
func (n *Node) Wait() {
	<-n.stop
}

// RegisterLifecycle registers the given Lifecycle on the node.
func (n *Node) RegisterLifecycle(lifecycle Lifecycle) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register lifecycle on running/stopped node")
	}
	if containsLifecycle(n.lifecycles, lifecycle) {
		panic(fmt.Sprintf("attempt to register lifecycle %T more than once", lifecycle))
	}
	n.lifecycles = append(n.lifecycles, lifecycle)
}

// RegisterAPIs registers the APIs a service provides on the node.
func (n *Node) RegisterAPIs(apis []rpc.API) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register APIs on running/stopped node")
	}
	n.rpcAPIs = append(n.rpcAPIs, apis...)
}

// RegisterHandler mounts a handler on the given path on the canonical HTTP server.
//
// The name of the handler is shown in a log message when the HTTP server starts
// and should be a descriptive term for the service provided by the handler.
func (n *Node) RegisterHandler(name, path string, handler http.Handler) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state != initializingState {
		panic("can't register HTTP handler on running/stopped node")
	}
	n.http.mux.Handle(path, handler)
	n.http.handlerNames[path] = name
}

// Attach creates an RPC client attached to an in-process API handler.
func (n *Node) Attach() (*rpc.Client, error) {
	return rpc.DialInProc(n.inprocHandler), nil
}

// RPCHandler returns the in-process RPC request handler.
func (n *Node) RPCHandler() (*rpc.Server, error) {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.state == closedState {
		return nil, ErrNodeStopped
	}
	return n.inprocHandler, nil
}

// Config returns the configuration of node.
func (n *Node) Config() *Config {
	return n.config
}

// DataDir retrieves the current datadir used by the protocol stack.
// Deprecated: No files should be stored in this directory, use InstanceDir instead.
func (n *Node) DataDir() string {
	return n.config.DataDir
}

// InstanceDir retrieves the instance directory used by the protocol stack.
func (n *Node) InstanceDir() string {
	return n.config.instanceDir()
}

// IPCEndpoint retrieves the current IPC endpoint used by the protocol stack.
func (n *Node) IPCEndpoint() string {
	return n.ipc.endpoint
}

// HTTPEndpoint returns the URL of the HTTP server.
func (n *Node) HTTPEndpoint() string {
	return "http://" + n.http.listenAddr()
}

// WSEndpoint retrieves the current WS endpoint used by the protocol stack.
func (n *Node) WSEndpoint() string {
	if n.http.wsAllowed() {
		return "ws://" + n.http.listenAddr()
	}
	return "ws://" + n.ws.listenAddr()
}

// EventMux retrieves the event multiplexer used by all the network services in
// the current protocol stack.
func (n *Node) EventMux() *event.TypeMux {
	return n.eventmux
}

// OpenDatabase opens an existing database with the given name (or creates one if no
// previous can be found) from within the node's instance directory. If the node is
// ephemeral, a memory database is returned.
func (n *Node) OpenDatabase(shardid uint64, name string, cache, handles int, namespace string) (ethdb.Database, error) {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.state == closedState {
		return nil, ErrNodeStopped
	}

	var db ethdb.Database
	var err error
	if n.config.DataDir == "" {
		db = rawdb.NewMemoryDatabase()
	} else {
		db, err = rawdb.NewLevelDBDatabase(n.ResolvePath(fmt.Sprintf("%s_%d", name, shardid)), cache, handles, namespace)
	}

	if err == nil {
		db = n.wrapDatabase(db)
	}
	return db, err
}

// ResolvePath returns the absolute path of a resource in the instance directory.
func (n *Node) ResolvePath(x string) string {
	return n.config.ResolvePath(x)
}

// closeTrackingDB wraps the Close method of a database. When the database is closed by the
// service, the wrapper removes it from the node's database map. This ensures that Node
// won't auto-close the database if it is closed by the service that opened it.
type closeTrackingDB struct {
	ethdb.Database
	n *Node
}

func (db *closeTrackingDB) Close() error {
	db.n.lock.Lock()
	delete(db.n.databases, db)
	db.n.lock.Unlock()
	return db.Database.Close()
}

// wrapDatabase ensures the database will be auto-closed when Node is closed.
func (n *Node) wrapDatabase(db ethdb.Database) ethdb.Database {
	wrapper := &closeTrackingDB{db, n}
	n.databases[wrapper] = struct{}{}
	return wrapper
}

// closeDatabases closes all open databases.
func (n *Node) closeDatabases() (errors []error) {
	for db := range n.databases {
		delete(n.databases, db)
		if err := db.Database.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}
