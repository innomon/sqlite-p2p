package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"crm-sqlite-pear-p2p/internal/config"
	"crm-sqlite-pear-p2p/internal/p2p"
	"crm-sqlite-pear-p2p/internal/store"
)

// BuildRootCommand constructs the top-level CLI command tree with core subcommands.
func BuildRootCommand(version string, cfg config.Config) *Command {
	return BuildRootCommandWithEngine(version, cfg, nil, nil)
}

// BuildRootCommandWithTracker constructs the top-level CLI command tree with tracker injection.
func BuildRootCommandWithTracker(version string, cfg config.Config, tracker *store.ChangesetTracker) *Command {
	return BuildRootCommandWithEngineAndTracker(version, cfg, nil, nil, tracker)
}

// BuildRootCommandWithEngine constructs the top-level CLI command tree with optional engine and swarm injection.
func BuildRootCommandWithEngine(version string, cfg config.Config, engine *p2p.ReplicationEngine, swarm *p2p.SwarmManager) *Command {
	return BuildRootCommandWithEngineAndTracker(version, cfg, engine, swarm, nil)
}

// BuildRootCommandWithEngineAndTracker constructs the top-level CLI command tree with full injection support.
func BuildRootCommandWithEngineAndTracker(version string, cfg config.Config, engine *p2p.ReplicationEngine, swarm *p2p.SwarmManager, tracker *store.ChangesetTracker) *Command {
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

	// customer commands
	customerCmd := NewCommand("customer", "Manage customer records and changesets")

	customerCmd.AddSubcommand(cliSubcommand(
		"put",
		"Insert or update a customer record: customer put <phone> <metadata_json> [payload]",
		"crm-peer customer put <phone> <metadata_json> [payload]",
		func(ctx context.Context, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: crm-peer customer put <phone> <metadata_json> [payload]")
			}
			t := tracker
			if t == nil {
				db, err := store.OpenDB(cfg.DBPath, cfg.EnableWAL)
				if err != nil {
					return err
				}
				defer db.Close()
				t = store.NewChangesetTracker(store.NewRepository(db))
			}

			key, err := store.FormatCustomerKey(args[0])
			if err != nil {
				return err
			}

			meta := json.RawMessage(args[1])
			var data []byte
			if len(args) > 2 {
				data = []byte(args[2])
			}

			if err := t.Put(ctx, key, meta, data); err != nil {
				return err
			}

			fmt.Fprintf(root.Stdout, "Saved customer record:\n  Key: %s\n", key)
			return nil
		},
	))

	customerCmd.AddSubcommand(cliSubcommand(
		"get",
		"Retrieve a customer record: customer get <phone_or_key>",
		"crm-peer customer get <phone_or_key>",
		func(ctx context.Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: crm-peer customer get <phone_or_key>")
			}

			target := args[0]
			key := target
			if !strings.HasPrefix(target, store.CustomerNamespace+":") {
				var err error
				key, err = store.FormatCustomerKey(target)
				if err != nil {
					return err
				}
			}

			db, err := store.OpenDB(cfg.DBPath, cfg.EnableWAL)
			if err != nil {
				return err
			}
			defer db.Close()
			repo := store.NewRepository(db)

			rec, err := repo.Get(ctx, key)
			if err != nil {
				return err
			}

			fmt.Fprintf(root.Stdout, "Key:      %s\n", rec.Key)
			fmt.Fprintf(root.Stdout, "Metadata: %s\n", string(rec.Metadata))
			if len(rec.Data) > 0 {
				fmt.Fprintf(root.Stdout, "Data:     %s\n", string(rec.Data))
			}
			return nil
		},
	))

	customerCmd.AddSubcommand(cliSubcommand(
		"list",
		"List customer records: customer list [limit] [offset]",
		"crm-peer customer list [limit] [offset]",
		func(ctx context.Context, args []string) error {
			db, err := store.OpenDB(cfg.DBPath, cfg.EnableWAL)
			if err != nil {
				return err
			}
			defer db.Close()
			repo := store.NewRepository(db)

			count, err := repo.Count(ctx, store.CustomerNamespace)
			if err != nil {
				return err
			}

			records, err := repo.List(ctx, store.CustomerNamespace, 50, 0)
			if err != nil {
				return err
			}

			fmt.Fprintf(root.Stdout, "Total Customers: %d\n", count)
			for _, r := range records {
				fmt.Fprintf(root.Stdout, "  - %s\n", r.Key)
			}
			return nil
		},
	))

	root.AddSubcommand(customerCmd)

	// peer command
	peerCmd := NewCommand("peer", "Inspect P2P swarm and peer connections")
	peerCmd.AddSubcommand(cliSubcommand(
		"status",
		"Display active P2P swarm and connection status",
		"crm-peer peer status",
		func(ctx context.Context, args []string) error {
			port := 0
			peers := 0
			topics := 0
			if swarm != nil {
				stats := swarm.Stats()
				port = stats.Port
				peers = stats.PeerCount
				topics = stats.ActiveTopics
			}
			feedLen := uint64(0)
			if engine != nil && engine.Feed() != nil {
				feedLen = engine.Feed().Len()
			}
			fmt.Fprintf(root.Stdout, "--- Peer Status ---\n")
			fmt.Fprintf(root.Stdout, "Port:            %d\n", port)
			fmt.Fprintf(root.Stdout, "Active Topics:   %d\n", topics)
			fmt.Fprintf(root.Stdout, "Connected Peers: %d\n", peers)
			fmt.Fprintf(root.Stdout, "Local Feed Len:  %d\n", feedLen)
			return nil
		},
	))
	peerCmd.AddSubcommand(cliSubcommand(
		"list",
		"List all connected P2P peers",
		"crm-peer peer list",
		func(ctx context.Context, args []string) error {
			count := 0
			if swarm != nil {
				count = swarm.PeerCount()
			}
			fmt.Fprintf(root.Stdout, "Connected Peers: %d\n", count)
			return nil
		},
	))
	root.AddSubcommand(peerCmd)

	// sync command
	syncCmd := cliSubcommand(
		"sync",
		"Trigger immediate peer reconciliation sweep",
		"crm-peer sync",
		func(ctx context.Context, args []string) error {
			count := 0
			if engine != nil {
				var err error
				count, err = engine.SyncPeers(ctx)
				if err != nil {
					return fmt.Errorf("sync failed: %w", err)
				}
			}
			fmt.Fprintf(root.Stdout, "Sync complete: %d changesets broadcast to connected peers\n", count)
			return nil
		},
	)
	root.AddSubcommand(syncCmd)


	// start command
	startCmd := cliSubcommand(
		"start",
		"Start the CRM peer replication node",
		"crm-peer start [--dry-run]",
		func(ctx context.Context, args []string) error {
			fmt.Fprintf(root.Stdout, "Starting P2P replication node on topic %s...\n", cfg.SwarmTopic)
			if len(args) > 0 && args[0] == "--dry-run" {
				return nil
			}

			// Background loop until context canceled
			<-ctx.Done()

			// Graceful cleanup
			if swarm != nil {
				_ = swarm.Close()
			}
			if engine != nil && engine.Feed() != nil {
				// Replicator cleanup if accessible or let caller manage
			}

			fmt.Fprintf(root.Stdout, "P2P replication node stopped.\n")
			return nil
		},
	)
	root.AddSubcommand(startCmd)


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
