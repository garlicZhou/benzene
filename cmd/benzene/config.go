package main

import (
	"fmt"
	"github.com/pelletier/go-toml"
	"github.com/spf13/cobra"
	"io/ioutil"
	"os"
)

type benzeneConfig struct {
	General generalConfig
	P2P     p2pConfig
	Pprof   pprofConfig
	Log     logConfig
}

type generalConfig struct {
	ShardID    int
	DataDir    string
}

type p2pConfig struct {
	Port    int
	IP      string
	KeyFile string
}

type pprofConfig struct {
	Enabled    bool
	ListenAddr string
}

type logConfig struct {
	Folder     string
	FileName   string
	RotateSize int
	Verbosity  int
	Context    *logContext `toml:",omitempty"`
}

type logContext struct {
	IP   string
	Port int
}

var dumpConfigCmd = &cobra.Command{
	Use:   "dumpconfig [config_file]",
	Short: "dump the config file for benzene binary configurations",
	Long:  "dump the config file for benzene binary configurations",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		config := getDefaultConfigCopy()

		if err := writeBenzeneConfigToFile(config, args[0]); err != nil {
			fmt.Println(err)
			os.Exit(128)
		}
	},
}

func loadBenzeneConfig(file string) (benzeneConfig, error) {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return benzeneConfig{}, err
	}

	var config benzeneConfig
	if err := toml.Unmarshal(b, &config); err != nil {
		return benzeneConfig{}, err
	}

	return config, nil
}

func writeBenzeneConfigToFile(config benzeneConfig, file string) error {
	b, err := toml.Marshal(config)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(file, b, 0644)
}
