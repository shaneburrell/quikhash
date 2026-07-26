package main

import (
	"os"

	"github.com/shaneburrell/quikhash/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
