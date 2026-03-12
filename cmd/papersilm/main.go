package main

import (
	"context"
	"os"

	"github.com/zzqDeco/papersilm/internal/cli"
)

func main() {
	cmd := cli.NewRootCommand(context.Background())
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
