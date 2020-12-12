package main

import (
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

	}

	rpcFlags = []cli.Flag{

	}

	metricsFlags = []cli.Flag{

	}
)

func init() {
	// Initialize the CLI app and start Benzene
	app.Action = benzene
	app.Commands = []cli.Command{

	}
	sort.Sort(cli.CommandsByName(app.Commands))
	app.Flags = append(app.Flags, nodeFlags...)
	app.Flags = append(app.Flags, rpcFlags...)
	app.Flags = append(app.Flags, consoleFlags...)
	app.Flags = append(app.Flags, metricsFlags...)
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

	stack.Wait()
	return nil
}

// startNode boots up the system node and all registered protocols, after which
// it unlocks any requested accounts, and starts the RPC/IPC interfaces and the
// miner.
func startNode(ctx *cli.Context, stack *node.Node, backend ethapi.Backend) {

}
