package main

import (
	"benzene/consensus"
	"benzene/core"
	"benzene/internal/genesis"
	"benzene/internal/utils"
	"benzene/node"
	"benzene/p2p"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	nodeconfig "benzene/internal/configs/node"
)

var (
	myHost          p2p.Host
	initialAccounts []*genesis.DeployAccount
)

var rootCmd = &cobra.Command{
	Use: "benzene",
	Run: runBenzeneNode,
}

var configFlag = StringFlag{
	Name:      "config",
	Usage:     "load node config from the config toml file.",
	Shorthand: "c",
	DefValue:  "",
}

func init() {
	SetParseErrorHandle(func(err error) {
		os.Exit(128) // 128 - invalid command line arguments
	})
	rootCmd.AddCommand(dumpConfigCmd)

	if err := registerRootCmdFlags(); err != nil {
		os.Exit(2)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func registerRootCmdFlags() error {
	flags := getRootFlags()

	return RegisterFlags(rootCmd, flags)
}

func runBenzeneNode(cmd *cobra.Command, args []string) {
	cfg, err := getBenzeneConfig(cmd)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		cmd.Help()
		os.Exit(128)
	}
	setupNodeLog(cfg)
	setupPprof(cfg)
	setupNodeAndRun(cfg)
}

func getBenzeneConfig(cmd *cobra.Command) (benzeneConfig, error) {
	var (
		config benzeneConfig
		err    error
	)
	if IsFlagChanged(cmd, configFlag) {
		configFile := GetStringFlagValue(cmd, configFlag)
		config, err = loadBenzeneConfig(configFile)
	} else {
		config = getDefaultConfigCopy()
	}
	if err != nil {
		return benzeneConfig{}, err
	}

	applyRootFlags(cmd, &config)

	return config, err
}

func applyRootFlags(cmd *cobra.Command, config *benzeneConfig) {
	applyGeneralFlags(cmd, config)
	applyP2PFlags(cmd, config)
	applyPprofFlags(cmd, config)
	applyLogFlags(cmd, config)
}

func setupNodeLog(config benzeneConfig) {
	logPath := filepath.Join(config.Log.Folder, config.Log.FileName)
	rotateSize := config.Log.RotateSize
	verbosity := config.Log.Verbosity

	utils.AddLogFile(logPath, rotateSize)
	utils.SetLogVerbosity(log.Lvl(verbosity))
	if config.Log.Context != nil {
		ip := config.Log.Context.IP
		port := config.Log.Context.Port
		utils.SetLogContext(ip, strconv.Itoa(port))
	}
}

func setupPprof(config benzeneConfig) {
	enabled := config.Pprof.Enabled
	addr := config.Pprof.ListenAddr

	if enabled {
		go func() {
			http.ListenAndServe(addr, nil)
		}()
	}
}

func setupNodeAndRun(config benzeneConfig) {

	nodeConfig, err := createGlobalConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR cannot configure node: %s\n", err)
		os.Exit(1)
	}
	currentNode := setupConsensusAndNode(config, nodeConfig)
	nodeconfig.GetDefaultConfig().ShardID = nodeConfig.ShardID

	// Prepare for graceful shutdown from os signals
	osSignal := make(chan os.Signal)
	signal.Notify(osSignal, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range osSignal {
			if sig == syscall.SIGTERM || sig == os.Interrupt {
				const msg = "Got %s signal. Gracefully shutting down...\n"
				utils.Logger().Printf(msg, sig)
				fmt.Printf(msg, sig)
				currentNode.ShutDown()
			}
		}
	}()

	startMsg := "==== New Benzene Node ===="
	utils.Logger().Info().
		Uint32("ShardID", nodeConfig.ShardID).
		Str("multiaddress",
			fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", config.P2P.IP, config.P2P.Port, myHost.GetID().Pretty()),
		).
		Msg(startMsg)

	nodeconfig.SetPeerID(myHost.GetID())

	if err := currentNode.Start(); err != nil {
		fmt.Println("could not begin network message handling for node", err.Error())
		os.Exit(-1)
	}

}

func createGlobalConfig(config benzeneConfig) (*nodeconfig.ConfigType, error) {
	var err error

	if len(initialAccounts) == 0 {
		initialAccounts = append(initialAccounts, &genesis.DeployAccount{ShardID: uint32(config.General.ShardID)})
	}
	nodeConfig := nodeconfig.GetShardConfig(initialAccounts[0].ShardID)

	// P2P private key is used for secure message transfer between p2p nodes.
	nodeConfig.P2PPriKey, _, err = utils.LoadKeyFromFile(config.P2P.KeyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot load or create P2P key at %#v",
			config.P2P.KeyFile)
	}

	selfPeer := p2p.Peer{
		IP:              config.P2P.IP,
		Port:            strconv.Itoa(config.P2P.Port),
	}

	myHost, err = p2p.NewHost(&selfPeer, nodeConfig.P2PPriKey)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create P2P network host")
	}

	nodeConfig.DBDir = config.General.DataDir

	return nodeConfig, nil
}

func setupConsensusAndNode(config benzeneConfig, nodeConfig *nodeconfig.ConfigType) *node.Node {
	// Consensus object.
	currentConsensus, err := consensus.New(
		myHost, nodeConfig.ShardID,
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error :%v \n", err)
		os.Exit(1)
	}

	// Current node.
	chainDBFactory := &core.LDBFactory{RootDir: nodeConfig.DBDir}

	currentNode := node.New(myHost, currentConsensus, chainDBFactory)

	// TODO: refactor the creation of blockchain out of node.New()
	currentConsensus.Blockchain = currentNode.Blockchain()

	nodeconfig.GetDefaultConfig().DBDir = nodeConfig.DBDir

	return currentNode
}