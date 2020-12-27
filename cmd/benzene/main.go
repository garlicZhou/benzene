package main

import (
	"benzene/cmd/utils"
	"benzene/console/prompt"
	"benzene/internal/bnzapi"
	"benzene/internal/debug"
	"benzene/internal/flags"
	"benzene/internal/genesis"
	"benzene/node"
	"benzene/p2p"
	"fmt"
	"gopkg.in/urfave/cli.v1"
	"os"
	"sort"
)

const (
	clientIdentifier = "benzene" // Client identifier to advertise over the network
)

var (
	myHost          p2p.Host
	initialAccounts []*genesis.DeployAccount
)

var (
	// Git SHA1 commit hash of the release (set via linker flags)
	gitCommit = ""
	gitDate   = ""
	// The app that holds all commands and flags.
	app = flags.NewApp(gitCommit, gitDate, "the benzene command line interface")
	// flags that configure the node
	nodeFlags = []cli.Flag{
		utils.DataDirFlag,
		utils.ShardIDFlag,
		utils.P2PPortFlag,
		utils.P2PIPFlag,
		utils.P2PKeyFileFlag,
	}

	rpcFlags = []cli.Flag{
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
		utils.HTTPEnabledFlag,
		utils.HTTPListenAddrFlag,
		utils.HTTPPortFlag,
		utils.HTTPApiFlag,
		utils.HTTPCORSDomainFlag,
		utils.HTTPVirtualHostsFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.WSPortFlag,
		utils.WSApiFlag,
		utils.WSAllowedOriginsFlag,
	}

	metricsFlags = []cli.Flag{}
)

func init() {
	// Initialize the CLI app and start Benzene
	app.Action = benzene
	app.Commands = []cli.Command{
		// See consolecmd.go:
		consoleCommand,
		attachCommand,
		javascriptCommand,
	}
	sort.Sort(cli.CommandsByName(app.Commands))
	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, debug.Flags...)
	app.Flags = append(app.Flags, consoleFlags...)
	app.Flags = append(app.Flags, metricsFlags...)

	app.Before = func(ctx *cli.Context) error {
		return debug.Setup(ctx)
	}
	app.After = func(ctx *cli.Context) error {
		debug.Exit()
		prompt.Stdin.Close() // Resets terminal mode.
		return nil
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// prepare manipulates memory cache allowance and setups metric system.
// This function should be called before launching devp2p stack.
func prepare(ctx *cli.Context) {

}

// benzene is the main entry point into the system if no special subcommand is ran.
// It creates a default node based on the command line arguments and runs it in
// blocking mode, waiting for it to be shut down.
func benzene(ctx *cli.Context) error {
	if args := ctx.Args(); len(args) > 0 {
		return fmt.Errorf("invalid command: %q", args[0])
	}

	prepare(ctx)
	stack, backend := makeFullNode(ctx)
	defer stack.Close()

	startNode(ctx, stack, backend)
	stack.Wait()
	return nil
}

// startNode boots up the system node and all registered protocols, after which
// it unlocks any requested accounts, and starts the RPC/IPC interfaces and the
// miner.
func startNode(ctx *cli.Context, stack *node.Node, backend bnzapi.Backend) {
	debug.Memsize.Add("node", stack)

	// Start up the node itself
	utils.StartNode(stack)
}
