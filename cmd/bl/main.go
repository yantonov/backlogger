// Command bl is the backlogger command-line tool.
package main

import (
	"os"

	"github.com/yantonov/backlogger/internal/cli"
)

func main() {
	os.Exit(cli.NewApp().Run(os.Args[1:]))
}
