package main

import (
	"os"

	"github.com/diasYuri/opera/internal/cli"
)

var version = "0.0.0-dev"

func main() {
	os.Exit(cli.New(os.Stdin, os.Stdout, os.Stderr, version).Run(os.Args[1:]))
}
