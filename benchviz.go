package main

import (
	"github.com/fredbi/benchviz/internal/cmd"
)

func main() {
	cli := cmd.NewCommand()

	// parse command line; exit if invalid
	if err := cli.Parse(); err != nil {
		cli.Fatalf(err)

		return
	}

	if err := cli.Execute(); err != nil {
		cli.Fatalf(err)
	}
}
