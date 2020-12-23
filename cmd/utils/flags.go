// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// Package utils contains internal helper functions for go-ethereum commands.
package utils

import (
	"benzene/bnz"
	"benzene/internal/bnzapi"
	"benzene/internal/configs"
	"benzene/internal/utils"
	"benzene/node"
	"github.com/ethereum/go-ethereum/common"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"

	"benzene/internal/flags"
	"gopkg.in/urfave/cli.v1"
)

func init() {
	cli.AppHelpTemplate = `{{.Name}} {{if .Flags}}[global options] {{end}}command{{if .Flags}} [command options]{{end}} [arguments...]

VERSION:
   {{.Version}}

COMMANDS:
   {{range .Commands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
   {{end}}{{if .Flags}}
GLOBAL OPTIONS:
   {{range .Flags}}{{.}}
   {{end}}{{end}}
`
	cli.CommandHelpTemplate = flags.CommandHelpTemplate
	cli.HelpPrinter = printHelp
}

func printHelp(out io.Writer, templ string, data interface{}) {
	funcMap := template.FuncMap{"join": strings.Join}
	t := template.Must(template.New("help").Funcs(funcMap).Parse(templ))
	w := tabwriter.NewWriter(out, 38, 8, 2, ' ', 0)
	err := t.Execute(w, data)
	if err != nil {
		panic(err)
	}
	w.Flush()
}

// These are all the command line flags we support.
// If you add to this list, please remember to include the
// flag in the appropriate command definition.
//
// The flags are defined here so their names and help texts
// are the same for all commands.

// General settings
var (
	DataDirFlag = DirectoryFlag{
		Name:  "datadir",
		Usage: "Data directory for the databases and keystore",
		Value: DirectoryString(node.DefaultDataDir()),
	}
)

// Console settings
var (
	ExecFlag = cli.StringFlag{
		Name:  "exec",
		Usage: "Execute JavaScript statement",
	}
	PreloadJSFlag = cli.StringFlag{
		Name:  "preload",
		Usage: "Comma separated list of JavaScript files to preload into the console",
	}
	// ATM the url is left to the user and deployment to
	JSpathFlag = cli.StringFlag{
		Name:  "jspath",
		Usage: "JavaScript root path for `loadScript`",
		Value: ".",
	}
)

// Sharding settings
var (
	ShardIDFlag = cli.StringFlag{
		Name:  "run.shard",
		Usage: "Comma separated shard IDs of the node",
		Value: "1",
	}
)

// P2P flags
var (
	P2PPortFlag = cli.StringFlag{
		Name:  "p2p.port",
		Usage: "port to listen for p2p protocols",
		Value: "9000",
	}
	P2PIPFlag = cli.StringFlag{
		Name:  "p2p.ip",
		Usage: "ip to listen for p2p protocols",
		Value: "0.0.0.0",
	}
	P2PKeyFileFlag = cli.StringFlag{
		Name:  "p2p.keyfile",
		Usage: "the p2p key file of the benzene node",
		Value: "./.bnzkey",
	}
)

// MakeDataDir retrieves the currently requested data directory, terminating
// if none (or the empty string) is specified. If the node is starting a testnet,
// then a subdirectory of the specified datadir will be used.
func MakeDataDir(ctx *cli.Context) string {
	if path := ctx.GlobalString(DataDirFlag.Name); path != "" {
		return path
	}
	Fatalf("Cannot determine default data directory, please set manually (--datadir)")
	return ""
}

func setDataDir(ctx *cli.Context, cfg *node.Config) {
	switch {
	case ctx.GlobalIsSet(DataDirFlag.Name):
		cfg.DataDir = ctx.GlobalString(DataDirFlag.Name)
	}
}

func SetP2PConfig(ctx *cli.Context, cfg *node.Config) {
	var err error
	cfg.Port = ctx.GlobalString(P2PPortFlag.Name)
	cfg.IP = ctx.GlobalString(P2PIPFlag.Name)
	// P2P private key is used for secure message transfer between p2p nodes.
	cfg.P2PPriKey, _, err = utils.LoadKeyFromFile(ctx.GlobalString(P2PKeyFileFlag.Name))
	if err != nil {
		Fatalf("cannot load or create P2P key at %#v: %v",
			ctx.GlobalString(P2PKeyFileFlag.Name), err)
	}
}

func SetShardConfig(ctx *cli.Context, cfg *node.Config) {
	shardidlist := ctx.GlobalString(ShardIDFlag.Name)
	sharidmap := make(map[uint64]bool)
	for _, entry := range strings.Split(shardidlist, ",") {
		shardid, err := strconv.ParseUint(entry, 10, 64)
		if err != nil {
			Fatalf("Invalid shard id %s: %v", entry, err)
		}
		if shardid > configs.MaxShards {
			Fatalf("Shard id %i is bigger than maximum shard %i", shardid, configs.MaxShards)
		}
		sharidmap[shardid] = true
	}
	// remove the repeated shard id and sort it
	for shardid := range sharidmap {
		cfg.ShardID = append(cfg.ShardID, shardid)
	}
	sort.Slice(cfg.ShardID, func(i, j int) bool { return cfg.ShardID[i] < cfg.ShardID[j] })
	for _, shardid := range cfg.ShardID {
		cfg.GroupID = append(cfg.GroupID, configs.NewGroupIDByShardID(shardid))
	}
}

// SetNodeConfig applies node-related command line flags to the config.
func SetNodeConfig(ctx *cli.Context, cfg *node.Config) {
	setDataDir(ctx, cfg)
	SetP2PConfig(ctx, cfg)
	SetShardConfig(ctx, cfg)
}

// SetEthConfig applies eth-related command line flags to the config.
func SetBnzConfig(ctx *cli.Context, stack *node.Node, cfg *bnz.Config) {

}

// RegisterBnzService adds an Benzene client to the stack.
func RegisterBnzService(stack *node.Node, cfg *bnz.Config) bnzapi.Backend {
	backend, err := bnz.New(stack, cfg)
	if err != nil {
		Fatalf("Failed to register the Benzene service: %v", err)
	}
	return backend.APIBackend
}

// MakeConsolePreloads retrieves the absolute paths for the console JavaScript
// scripts to preload before starting.
func MakeConsolePreloads(ctx *cli.Context) []string {
	// Skip preloading if there's nothing to preload
	if ctx.GlobalString(PreloadJSFlag.Name) == "" {
		return nil
	}
	// Otherwise resolve absolute paths and return them
	var preloads []string

	assets := ctx.GlobalString(JSpathFlag.Name)
	for _, file := range strings.Split(ctx.GlobalString(PreloadJSFlag.Name), ",") {
		preloads = append(preloads, common.AbsolutePath(assets, strings.TrimSpace(file)))
	}
	return preloads
}

// MigrateFlags sets the global flag from a local flag when it's set.
// This is a temporary function used for migrating old command/flags to the
// new format.
//
// e.g. geth account new --keystore /tmp/mykeystore --lightkdf
//
// is equivalent after calling this method with:
//
// geth --keystore /tmp/mykeystore --lightkdf account new
//
// This allows the use of the existing configuration functionality.
// When all flags are migrated this function can be removed and the existing
// configuration functionality must be changed that is uses local flags
func MigrateFlags(action func(ctx *cli.Context) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				ctx.GlobalSet(name, ctx.String(name))
			}
		}
		return action(ctx)
	}
}
