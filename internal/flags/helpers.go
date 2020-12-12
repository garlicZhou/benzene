package flags

import (
	"github.com/ethereum/go-ethereum/params"
	"gopkg.in/urfave/cli.v1"
	"os"
	"path/filepath"
)

// NewApp creates an app with sane defaults.
func NewApp(gitCommit, gitDate, usage string) *cli.App {
	app := cli.NewApp()
	app.Name = filepath.Base(os.Args[0])
	app.Author = ""
	app.Email = ""
	app.Version = params.VersionWithCommit(gitCommit, gitDate)
	app.Usage = usage
	return app
}
