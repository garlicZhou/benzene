package main

var defaultConfig = benzeneConfig{
	General: generalConfig{
		ShardID:    -1,
		DataDir:    "./",
	},
	P2P: p2pConfig{
		Port:    9000,
		IP:      "0.0.0.0",
		KeyFile: "./.bznkey",
	},
	Pprof: pprofConfig{
		Enabled:    false,
		ListenAddr: "127.0.0.1:6060",
	},
	Log: logConfig{
		Folder:     "./latest",
		FileName:   "benzene.log",
		RotateSize: 100,
		Verbosity:  3,
	},
}

var defaultLogContext = logContext{
	IP:   "127.0.0.1",
	Port: 9000,
}

func getDefaultConfigCopy() benzeneConfig {
	return defaultConfig
}

func getDefaultLogContextCopy() logContext {
	config := defaultLogContext
	return config
}
