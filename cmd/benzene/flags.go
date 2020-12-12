package main

import (
	"github.com/spf13/cobra"
)

var (

	generalFlags = []Flag{
		shardIDFlag,
		dataDirFlag,

		legacyShardIDFlag,
		legacyDataDirFlag,
	}

	p2pFlags = []Flag{
		p2pPortFlag,
		p2pIPFlag,
		p2pKeyFileFlag,

		legacyKeyFileFlag,
	}

	pprofFlags = []Flag{
		pprofEnabledFlag,
		pprofListenAddrFlag,
	}

	logFlags = []Flag{
		logFolderFlag,
		logRotateSizeFlag,
		logFileNameFlag,
		logContextIPFlag,
		logContextPortFlag,
		logVerbosityFlag,
		legacyVerbosityFlag,

		legacyLogFolderFlag,
		legacyLogRotateSizeFlag,
	}
)

func getRootFlags() []Flag {
	var flags []Flag

	flags = append(flags, configFlag)
	flags = append(flags, generalFlags...)
	flags = append(flags, p2pFlags...)
	flags = append(flags, logFlags...)

	return flags
}

// general flags
var (
	shardIDFlag = IntFlag{
		Name:     "run.shard",
		Usage:    "run node on the given shard ID (-1 automatically configured by BLS keys)",
		DefValue: defaultConfig.General.ShardID,
	}
	dataDirFlag = StringFlag{
		Name:     "datadir",
		Usage:    "directory of chain database",
		DefValue: defaultConfig.General.DataDir,
	}
	legacyShardIDFlag = IntFlag{
		Name:       "shard_id",
		Usage:      "the shard ID of this node",
		DefValue:   defaultConfig.General.ShardID,
		Deprecated: "use --run.shard",
	}
	legacyDataDirFlag = StringFlag{
		Name:       "db_dir",
		Usage:      "blockchain database directory",
		DefValue:   defaultConfig.General.DataDir,
		Deprecated: "use --datadir",
	}
)

func applyGeneralFlags(cmd *cobra.Command, config *benzeneConfig) {
	if IsFlagChanged(cmd, shardIDFlag) {
		config.General.ShardID = GetIntFlagValue(cmd, shardIDFlag)
	} else if IsFlagChanged(cmd, legacyShardIDFlag) {
		config.General.ShardID = GetIntFlagValue(cmd, legacyShardIDFlag)
	}

	if IsFlagChanged(cmd, dataDirFlag) {
		config.General.DataDir = GetStringFlagValue(cmd, dataDirFlag)
	} else if IsFlagChanged(cmd, legacyDataDirFlag) {
		config.General.DataDir = GetStringFlagValue(cmd, legacyDataDirFlag)
	}

}

// p2p flags
var (
	p2pPortFlag = IntFlag{
		Name:     "p2p.port",
		Usage:    "port to listen for p2p protocols",
		DefValue: defaultConfig.P2P.Port,
	}
	p2pIPFlag = StringFlag{
		Name:     "p2p.ip",
		Usage:    "ip to listen for p2p protocols",
		DefValue: defaultConfig.P2P.IP,
	}
	p2pKeyFileFlag = StringFlag{
		Name:     "p2p.keyfile",
		Usage:    "the p2p key file of the harmony node",
		DefValue: defaultConfig.P2P.KeyFile,
	}
	legacyKeyFileFlag = StringFlag{
		Name:       "key",
		Usage:      "the p2p key file of the harmony node",
		DefValue:   defaultConfig.P2P.KeyFile,
		Deprecated: "use --p2p.keyfile",
	}
)

func applyP2PFlags(cmd *cobra.Command, config *benzeneConfig) {
	if IsFlagChanged(cmd, p2pPortFlag) {
		config.P2P.Port = GetIntFlagValue(cmd, p2pPortFlag)
	}
	if IsFlagChanged(cmd, p2pIPFlag) {
		config.P2P.IP = GetStringFlagValue(cmd, p2pIPFlag)
	}

	if IsFlagChanged(cmd, p2pKeyFileFlag) {
		config.P2P.KeyFile = GetStringFlagValue(cmd, p2pKeyFileFlag)
	} else if IsFlagChanged(cmd, legacyKeyFileFlag) {
		config.P2P.KeyFile = GetStringFlagValue(cmd, legacyKeyFileFlag)
	}
}

// http flags
var (
	httpEnabledFlag = BoolFlag{
		Name:     "http",
		Usage:    "enable HTTP / RPC requests",
		DefValue: defaultConfig.HTTP.Enabled,
	}
	httpIPFlag = StringFlag{
		Name:     "http.ip",
		Usage:    "ip address to listen for RPC calls. Use 0.0.0.0 for public endpoint",
		DefValue: defaultConfig.HTTP.IP,
	}
	httpPortFlag = IntFlag{
		Name:     "http.port",
		Usage:    "rpc port to listen for HTTP requests",
		DefValue: defaultConfig.HTTP.Port,
	}
)

func applyHTTPFlags(cmd *cobra.Command, config *benzeneConfig) {
	var isRPCSpecified bool

	if IsFlagChanged(cmd, httpIPFlag) {
		config.HTTP.IP = GetStringFlagValue(cmd, httpIPFlag)
		isRPCSpecified = true
	}

	if IsFlagChanged(cmd, httpPortFlag) {
		config.HTTP.Port = GetIntFlagValue(cmd, httpPortFlag)
		isRPCSpecified = true
	}

	if IsFlagChanged(cmd, httpEnabledFlag) {
		config.HTTP.Enabled = GetBoolFlagValue(cmd, httpEnabledFlag)
	} else if isRPCSpecified {
		config.HTTP.Enabled = true
	}

}

// ws flags
var (
	wsEnabledFlag = BoolFlag{
		Name:     "ws",
		Usage:    "enable websocket endpoint",
		DefValue: defaultConfig.WS.Enabled,
	}
	wsIPFlag = StringFlag{
		Name:     "ws.ip",
		Usage:    "ip endpoint for websocket. Use 0.0.0.0 for public endpoint",
		DefValue: defaultConfig.WS.IP,
	}
	wsPortFlag = IntFlag{
		Name:     "ws.port",
		Usage:    "port for websocket endpoint",
		DefValue: defaultConfig.WS.Port,
	}
)

func applyWSFlags(cmd *cobra.Command, config *benzeneConfig) {
	if IsFlagChanged(cmd, wsEnabledFlag) {
		config.WS.Enabled = GetBoolFlagValue(cmd, wsEnabledFlag)
	}
	if IsFlagChanged(cmd, wsIPFlag) {
		config.WS.IP = GetStringFlagValue(cmd, wsIPFlag)
	}
	if IsFlagChanged(cmd, wsPortFlag) {
		config.WS.Port = GetIntFlagValue(cmd, wsPortFlag)
	}
}

// pprof flags
var (
	pprofEnabledFlag = BoolFlag{
		Name:     "pprof",
		Usage:    "enable pprof profiling",
		DefValue: defaultConfig.Pprof.Enabled,
	}
	pprofListenAddrFlag = StringFlag{
		Name:     "pprof.addr",
		Usage:    "listen address for pprof",
		DefValue: defaultConfig.Pprof.ListenAddr,
	}
)

func applyPprofFlags(cmd *cobra.Command, config *benzeneConfig) {
	var pprofSet bool
	if IsFlagChanged(cmd, pprofListenAddrFlag) {
		config.Pprof.ListenAddr = GetStringFlagValue(cmd, pprofListenAddrFlag)
		pprofSet = true
	}
	if IsFlagChanged(cmd, pprofEnabledFlag) {
		config.Pprof.Enabled = GetBoolFlagValue(cmd, pprofEnabledFlag)
	} else if pprofSet {
		config.Pprof.Enabled = true
	}
}

// log flags
var (
	logFolderFlag = StringFlag{
		Name:     "log.dir",
		Usage:    "directory path to put rotation logs",
		DefValue: defaultConfig.Log.Folder,
	}
	logRotateSizeFlag = IntFlag{
		Name:     "log.max-size",
		Usage:    "rotation log size in megabytes",
		DefValue: defaultConfig.Log.RotateSize,
	}
	logFileNameFlag = StringFlag{
		Name:     "log.name",
		Usage:    "log file name (e.g. harmony.log)",
		DefValue: defaultConfig.Log.FileName,
	}
	logVerbosityFlag = IntFlag{
		Name:      "log.verb",
		Shorthand: "v",
		Usage:     "logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		DefValue:  defaultConfig.Log.Verbosity,
	}
	// TODO: remove context (this shall not be in the log)
	logContextIPFlag = StringFlag{
		Name:     "log.ctx.ip",
		Usage:    "log context ip",
		DefValue: defaultLogContext.IP,
		Hidden:   true,
	}
	logContextPortFlag = IntFlag{
		Name:     "log.ctx.port",
		Usage:    "log context port",
		DefValue: defaultLogContext.Port,
		Hidden:   true,
	}
	legacyLogFolderFlag = StringFlag{
		Name:       "log_folder",
		Usage:      "the folder collecting the logs of this execution",
		DefValue:   defaultConfig.Log.Folder,
		Deprecated: "use --log.path",
	}
	legacyLogRotateSizeFlag = IntFlag{
		Name:       "log_max_size",
		Usage:      "the max size in megabytes of the log file before it gets rotated",
		DefValue:   defaultConfig.Log.RotateSize,
		Deprecated: "use --log.max-size",
	}
	legacyVerbosityFlag = IntFlag{
		Name:       "verbosity",
		Usage:      "logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail",
		DefValue:   defaultConfig.Log.Verbosity,
		Deprecated: "use --log.verbosity",
	}
)

func applyLogFlags(cmd *cobra.Command, config *benzeneConfig) {
	if IsFlagChanged(cmd, logFolderFlag) {
		config.Log.Folder = GetStringFlagValue(cmd, logFolderFlag)
	} else if IsFlagChanged(cmd, legacyLogFolderFlag) {
		config.Log.Folder = GetStringFlagValue(cmd, legacyLogFolderFlag)
	}

	if IsFlagChanged(cmd, logRotateSizeFlag) {
		config.Log.RotateSize = GetIntFlagValue(cmd, logRotateSizeFlag)
	} else if IsFlagChanged(cmd, legacyLogRotateSizeFlag) {
		config.Log.RotateSize = GetIntFlagValue(cmd, legacyLogRotateSizeFlag)
	}

	if IsFlagChanged(cmd, logFileNameFlag) {
		config.Log.FileName = GetStringFlagValue(cmd, logFileNameFlag)
	}

	if IsFlagChanged(cmd, logVerbosityFlag) {
		config.Log.Verbosity = GetIntFlagValue(cmd, logVerbosityFlag)
	} else if IsFlagChanged(cmd, legacyVerbosityFlag) {
		config.Log.Verbosity = GetIntFlagValue(cmd, legacyVerbosityFlag)
	}

	if HasFlagsChanged(cmd, []Flag{logContextIPFlag, logContextPortFlag}) {
		ctx := getDefaultLogContextCopy()
		config.Log.Context = &ctx

		if IsFlagChanged(cmd, logContextIPFlag) {
			config.Log.Context.IP = GetStringFlagValue(cmd, logContextIPFlag)
		}
		if IsFlagChanged(cmd, logContextPortFlag) {
			config.Log.Context.Port = GetIntFlagValue(cmd, logContextPortFlag)
		}
	}
}