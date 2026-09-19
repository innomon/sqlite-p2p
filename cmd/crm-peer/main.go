package main

import (
	"context"
	"fmt"
	"os"

	"crm-sqlite-pear-p2p/internal/cli"
	"crm-sqlite-pear-p2p/internal/config"
	"crm-sqlite-pear-p2p/internal/logger"
)

// Version represents the application version.
var Version = "0.1.0"

func run(args []string) error {
	var explicitConfig string
	var filteredArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" && i+1 < len(args) {
			explicitConfig = args[i+1]
			i++
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	cfg, err := config.Load(explicitConfig)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	_, _ = logger.Init(logger.Config{
		Level:          cfg.LogLevel,
		ConsoleEnabled: false,
		FileEnabled:    true,
		Dir:            cfg.StorageDir,
		FileName:       "crm-peer.jsonl",
		MaxSizeMB:      10,
		MaxBackups:     3,
	})

	root := cli.BuildRootCommand(Version, cfg)
	return root.Dispatch(context.Background(), filteredArgs)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
