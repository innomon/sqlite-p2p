package cli

import (
	"context"
	"fmt"

	"crm-sqlite-pear-p2p/internal/config"
)

// BuildRootCommand constructs the top-level CLI command tree with core subcommands.
func BuildRootCommand(version string, cfg config.Config) *Command {
	root := NewCommand("crm-peer", "Distributed Multimodal Agentic CRM (Pure Go Pear/P2P)")

	root.Run = func(ctx context.Context, args []string) error {
		if len(args) > 0 && (args[0] == "-v" || args[0] == "--version") {
			fmt.Fprintf(root.Stdout, "crm-peer v%s\n", version)
			return nil
		}
		fmt.Fprint(root.Stdout, root.Help())
		return nil
	}

	// version command
	versionCmd := cliSubcommand(
		"version",
		"Print the binary version",
		"crm-peer version",
		func(ctx context.Context, args []string) error {
			fmt.Fprintf(root.Stdout, "crm-peer v%s\n", version)
			return nil
		},
	)
	root.AddSubcommand(versionCmd)

	// status command
	statusCmd := cliSubcommand(
		"status",
		"Show local node and database status",
		"crm-peer status",
		func(ctx context.Context, args []string) error {
			fmt.Fprintf(root.Stdout, "--- Node Status ---\n")
			fmt.Fprintf(root.Stdout, "Version:     v%s\n", version)
			fmt.Fprintf(root.Stdout, "DB Path:     %s\n", cfg.DBPath)
			fmt.Fprintf(root.Stdout, "WAL Mode:    %v\n", cfg.EnableWAL)
			fmt.Fprintf(root.Stdout, "Storage Dir: %s\n", cfg.StorageDir)
			fmt.Fprintf(root.Stdout, "Swarm Topic: %s\n", cfg.SwarmTopic)
			fmt.Fprintf(root.Stdout, "Log Level:   %s\n", cfg.LogLevel)
			return nil
		},
	)
	root.AddSubcommand(statusCmd)

	return root
}

func cliSubcommand(name, desc, usage string, fn HandlerFunc) *Command {
	return &Command{
		Name:        name,
		Description: desc,
		Usage:       usage,
		Run:         fn,
	}
}
