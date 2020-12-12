package node

import (
	msg_pb "benzene/api/proto/message"
	proto_node "benzene/api/proto/node"
	"benzene/api/service"
	"benzene/consensus"
	"benzene/core"
	"benzene/core/shardchain"
	"benzene/core/types"
	"benzene/internal/chain"
	nodeconfig "benzene/internal/configs/node"
	"benzene/internal/utils"
	"benzene/p2p"
	"benzene/params"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// Node represents a protocol-participating node in the network
type Node struct {
	Consensus             *consensus.Consensus // Consensus object containing all Consensus related data (e.g. committee members, signatures, commits)
	BlockChannel          chan *types.Block    // The channel to send newly proposed blocks
	ConfirmedBlockChannel chan *types.Block    // The channel to send confirmed blocks

	shardChains shardchain.Collection // Shard databases
	SelfPeer    p2p.Peer

	Neighbors sync.Map // All the neighbor nodes, key is the sha256 of Peer IP/Port, value is the p2p.Peer

	TxPool *core.TxPool

	// Syncing component.
	syncID [SyncIDLength]byte // a unique ID for the node during the state syncing process with peers

	host           p2p.Host         // The p2p host used to send/receive p2p messages
	serviceManager *service.Manager // Service manager.

	config      *Config
	chainConfig params.ChainConfig     // Chain configuration.

	isFirstTime bool // the node was started with a fresh database

	log           log.Logger
	stop          chan struct{} // Channel to wait for termination notifications
	startStopLock sync.Mutex    // Start/Stop are protected by an additional lock
	state         int           // Tracks state of node lifecycle

	lock          sync.Mutex
	rpcAPIs       []rpc.API   // List of APIs currently provided by the node
	http          *httpServer //
	ws            *httpServer //
	ipc           *ipcServer  // Stores information about the ipc http server
	inprocHandler *rpc.Server // In-process RPC request handler to process the API requests

}

const (
	initializingState = iota
	runningState
	closedState
)

// Blockchain returns the blockchain for the node's current shard.
func (node *Node) Blockchain() *core.BlockChain {
	shardID := node.config.ShardID
	bc, err := node.shardChains.ShardChain(shardID)
	if err != nil {
		utils.Logger().Error().
			Uint32("shardID", shardID).
			Err(err).
			Msg("cannot get shard chain")
	}
	return bc
}

// TODO: make this batch more transactions (hongzicong)
// Broadcast the transaction to nodes with the topic shardGroupID
func (node *Node) tryBroadcast(tx *types.Transaction) {
	msg := proto_node.ConstructTransactionListMessageAccount(types.Transactions{tx})

	shardGroupID := nodeconfig.NewGroupIDByShardID(nodeconfig.ShardID(tx.ShardID()))
	utils.Logger().Info().Str("shardGroupID", string(shardGroupID)).Msg("tryBroadcast")

	for attempt := 0; attempt < NumTryBroadCast; attempt++ {
		if err := node.host.SendMessageToGroups([]nodeconfig.GroupID{shardGroupID},
			p2p.ConstructMessage(msg)); err != nil {
			utils.Logger().Error().Int("attempt", attempt).Msg("Error when trying to broadcast tx")
		} else {
			break
		}
	}
}

// AddPendingTransaction adds one new transaction to the pending transaction list.
// This is only called from SDK.
func (node *Node) AddPendingTransaction(newTx *types.Transaction) error {
	if newTx.ShardID() == node.config.ShardID {
		errs := node.addPendingTransactions(types.Transactions{newTx})
		var err error
		for i := range errs {
			if errs[i] != nil {
				utils.Logger().Info().Err(errs[i]).Msg("[AddPendingTransaction] Failed adding new transaction")
				err = errs[i]
				break
			}
		}
		if err == nil {
			utils.Logger().Info().Str("Hash", newTx.Hash().Hex()).Msg("Broadcasting Tx")
			node.tryBroadcast(newTx)
		}
		return err
	}
	return errors.New("shard do not match")
}

// Add new transactions to the pending transaction list.
func (node *Node) addPendingTransactions(newTxs types.Transactions) []error {
	poolTxs := types.Transactions{}
	errs := []error{}
	for _, tx := range newTxs {
		// TODO: change this validation rule according to the cross-shard mechanism (hongzicong)
		if tx.ShardID() != tx.ToShardID() {
			errs = append(errs, errors.New("cross-shard tx not accepted yet"))
			continue
		}
		poolTxs = append(poolTxs, tx)
	}
	errs = append(errs, node.TxPool.AddRemotes(poolTxs)...)

	pendingCount, queueCount := node.TxPool.Stats()
	utils.Logger().Info().
		Interface("err", errs).
		Int("length of newTxs", len(newTxs)).
		Int("totalPending", pendingCount).
		Int("totalQueued", queueCount).
		Msg("[addPendingTransactions] Adding more transactions")
	return errs
}

type withError struct {
	err     error
	payload interface{}
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
	n.lock.Unlock()

	// Check if networking startup failed.
	if err != nil {
		n.doClose(nil)
		return err
	}

	// groupID and whether this topic is used for consensus
	type t struct {
		tp    nodeconfig.GroupID
		isCon bool
	}
	groups := map[nodeconfig.GroupID]bool{}

	// three topic subscribed by each validator
	for _, t := range []t{
		{n.config.GetShardGroupID(), true},
		{n.config.GetClientGroupID(), false},
	} {
		if _, ok := groups[t.tp]; !ok {
			groups[t.tp] = t.isCon
		}
	}

	type u struct {
		p2p.NamedTopic
		consensusBound bool
	}

	var allTopics []u

	utils.Logger().Debug().
		Interface("topics-ended-up-with", groups).
		Uint32("shard-id", n.Consensus.ShardID).
		Msg("starting with these topics")

	for key, isCon := range groups {
		topicHandle, err := n.host.GetOrJoin(string(key))
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

	// p2p consensus message handler function
	type p2pHandlerConsensus func(
		ctx context.Context,
		msg *msg_pb.Message,
	) error

	errChan := make(chan withError, 100)

	for e := range errChan {
		utils.SampledLogger().Info().
			Interface("item", e.payload).
			Msgf("[p2p]: issue while handling incoming p2p message: %v", e.err)
	}

	return nil
}

// GetSyncID returns the syncID of this node
func (node *Node) GetSyncID() [SyncIDLength]byte {
	return node.syncID
}

// New creates a new P2P node, ready for protocol registration.
func New(
	host p2p.Host,
	consensusObj *consensus.Consensus,
	chainDBFactory shardchain.DBFactory,
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
		log:           conf.Logger,
		stop:          make(chan struct{}),
	}

	// Register built-in APIs.
	node.rpcAPIs = append(node.rpcAPIs, node.apis()...)

	if consensusObj != nil {
		node.config = nodeconfig.GetShardConfig(consensusObj.ShardID)
	} else {
		node.config = nodeconfig.GetDefaultConfig()
	}

	if host != nil {
		node.host = host
		node.SelfPeer = host.GetSelfPeer()
	}

	chainConfig := params.ChainConfig{}
	node.chainConfig = chainConfig

	engine := chain.New()

	collection := shardchain.NewCollection(
		chainDBFactory, &genesisInitializer{&node}, engine, nil, &chainConfig,
	)
	node.shardChains = collection

	if host != nil && consensusObj != nil {
		// Consensus and associated channel to communicate blocks
		node.Consensus = consensusObj

		// Load the chains.
		blockchain := node.Blockchain() // this also sets node.isFirstTime if the DB is fresh

		if blockchain == nil {
			var err error
			shardID := node.config.ShardID
			// HACK get the real error reason
			_, err = node.shardChains.ShardChain(shardID)
			fmt.Fprintf(os.Stderr, "Cannot initialize node: %v\n", err)
			os.Exit(-1)
		}

		node.BlockChannel = make(chan *types.Block)
		node.ConfirmedBlockChannel = make(chan *types.Block)

		txPoolConfig := core.DefaultTxPoolConfig
		txPoolConfig.Journal = fmt.Sprintf("%v/%v", node.config.DBDir, txPoolConfig.Journal)
		node.TxPool = core.NewTxPool(txPoolConfig, node.Blockchain().Config(), blockchain)

	}

	utils.Logger().Info().
		Interface("genesis block header", node.Blockchain().GetHeaderByNumber(0)).
		Msg("Genesis block hash")

	// Configure RPC servers.
	node.http = newHTTPServer(node.log, conf.HTTPTimeouts)
	node.ws = newHTTPServer(node.log, rpc.DefaultHTTPTimeouts)
	node.ipc = newIPCServer(node.log, conf.IPCEndpoint())

	return &node, nil
}

// AddPeers adds neighbors nodes
func (node *Node) AddPeers(peers []*p2p.Peer) int {
	for _, p := range peers {
		key := fmt.Sprintf("%s:%s:%s", p.IP, p.Port, p.PeerID)
		_, ok := node.Neighbors.LoadOrStore(key, *p)
		if !ok {
			// !ok means new peer is stored
			node.host.AddPeer(p)
			continue
		}
	}

	return node.host.GetPeerCount()
}

func (node *Node) initNodeConfiguration() (service.NodeConfig, chan p2p.Peer, error) {
	chanPeer := make(chan p2p.Peer)
	nodeConfig := service.NodeConfig{
		ShardGroupID: node.config.GetShardGroupID(),
		Actions:      map[nodeconfig.GroupID]nodeconfig.ActionType{},
	}

	groups := []nodeconfig.GroupID{
		node.config.GetShardGroupID(),
		node.config.GetClientGroupID(),
	}

	// force the side effect of topic join
	if err := node.host.SendMessageToGroups(groups, []byte{}); err != nil {
		return nodeConfig, nil, err
	}

	return nodeConfig, chanPeer, nil
}

// ServiceManager ...
func (node *Node) ServiceManager() *service.Manager {
	return node.serviceManager
}

// ShutDown gracefully shut down the node server and dump the in-memory blockchain state into DB.
func (node *Node) ShutDown() {
	node.Blockchain().Stop()
	const msg = "Successfully shut down!\n"
	utils.Logger().Print(msg)
	fmt.Print(msg)
	os.Exit(0)
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
