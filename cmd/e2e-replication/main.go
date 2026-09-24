package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sqlite-p2p/pkg/p2p"
)

// NodeConfig defines the configuration parameters for a replication node.
type NodeConfig struct {
	NodeID       string   `json:"node_id"`
	SwarmPort    int      `json:"swarm_port"` // 0 = dynamic ephemeral port (Pear P2P default)
	Bootstrap    []string `json:"bootstrap"`  // HyperDHT bootstrap addresses
	PeerAddrs    []string `json:"peer_addrs"` // Optional direct peer addresses (e.g. ["192.168.1.100:43219"])
	DBPath       string   `json:"db_path"`
	SwarmTopic   string   `json:"swarm_topic"`
	EnableWAL    bool     `json:"enable_wal"`
	EnableCrypto bool     `json:"enable_crypto"`
	AutoSync     bool     `json:"auto_sync"`
}

// Command represents a handcrafted CLI or slash command.
type Command struct {
	Name        string
	Usage       string
	Description string
	Run         func(ctx context.Context, app *ReplicationApp, args []string) error
}

// CommandRegistry implements a handcrafted command registry without spf13/Cobra.
type CommandRegistry struct {
	commands map[string]Command
}

// NewCommandRegistry creates a new command registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{commands: make(map[string]Command)}
}

// Register registers a command.
func (cr *CommandRegistry) Register(cmd Command) {
	cr.commands[cmd.Name] = cmd
	// Also register with leading slash for slash command ergonomics
	if !strings.HasPrefix(cmd.Name, "/") {
		slashCmd := cmd
		slashCmd.Name = "/" + cmd.Name
		cr.commands[slashCmd.Name] = slashCmd
	}
}

// Execute looks up and runs a command by line string.
func (cr *CommandRegistry) Execute(ctx context.Context, app *ReplicationApp, line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	parts := strings.Fields(trimmed)
	cmdName := parts[0]
	args := parts[1:]

	cmd, ok := cr.commands[cmdName]
	if !ok {
		return fmt.Errorf("unknown command: %q (type 'help' for available commands)", cmdName)
	}

	return cmd.Run(ctx, app, args)
}

// ReplicationApp encapsulates the running node runtime.
type ReplicationApp struct {
	cfg      NodeConfig
	engine   *p2p.Engine
	logger   *slog.Logger
	registry *CommandRegistry
	stopChan chan struct{}
}

func main() {
	configPath := flag.String("config", "", "Path to JSON configuration file")
	nodeID := flag.String("node", "", "Node ID (e.g. node-linux, node-macos)")
	swarmPort := flag.Int("port", 0, "Swarm listening port (0 = dynamic ephemeral port, no static open port needed)")
	bootstrapAddr := flag.String("bootstrap", "", "DHT bootstrap node address (e.g. 192.168.1.100:43219)")
	peerAddr := flag.String("peer", "", "Direct peer address to connect to (e.g. 192.168.1.100:43219)")
	dbPath := flag.String("db", "", "SQLite database file path (e.g. data/node1.db)")
	cmdExec := flag.String("cmd", "", "One-shot command to run and exit (e.g. 'put user:1 Alice')")
	flag.Parse()

	cfg := NodeConfig{
		NodeID:       "node-peer",
		SwarmPort:    0, // 0 = dynamic ephemeral port (no static open port required)
		DBPath:       "data/node.db",
		SwarmTopic:   "sqlite-p2p-e2e-cluster",
		EnableWAL:    true,
		EnableCrypto: false,
		AutoSync:     true,
	}

	if *configPath != "" {
		fileData, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading config file %s: %v\n", *configPath, err)
			os.Exit(1)
		}
		if err := json.Unmarshal(fileData, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing config JSON: %v\n", err)
			os.Exit(1)
		}
	}

	// Override with CLI flags if provided
	if *nodeID != "" {
		cfg.NodeID = *nodeID
	}
	if *swarmPort != 0 {
		cfg.SwarmPort = *swarmPort
	}
	if *bootstrapAddr != "" {
		cfg.Bootstrap = append(cfg.Bootstrap, *bootstrapAddr)
	}
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}
	if *peerAddr != "" {
		cfg.PeerAddrs = append(cfg.PeerAddrs, *peerAddr)
	}

	// Ensure DB directory exists
	if dir := filepath.Dir(cfg.DBPath); dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}

	// If no bootstrap nodes and no direct peers were specified, attempt LAN UDP broadcast discovery
	if len(cfg.Bootstrap) == 0 && len(cfg.PeerAddrs) == 0 {
		discCtx, discCancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
		lanPeer, err := p2p.DiscoverLANBootstrap(discCtx, "sqlite-p2p", cfg.SwarmTopic, p2p.DefaultBeaconPort, 600*time.Millisecond)
		discCancel()
		if err == nil && lanPeer != "" {
			fmt.Printf("[LAN Auto-Discovery] Found active local cluster seed at %s! Auto-joining...\n", lanPeer)
			cfg.Bootstrap = append(cfg.Bootstrap, lanPeer)
		}
	}

	// Topic hash
	topicHash := sha256.Sum256([]byte(cfg.SwarmTopic))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	engine, err := p2p.OpenEngine(p2p.EngineOptions{
		DBPath:       cfg.DBPath,
		EnableWAL:    cfg.EnableWAL,
		EnableCrypto: cfg.EnableCrypto,
		SwarmTopic:   topicHash,
		SwarmPort:    cfg.SwarmPort,
		Bootstrap:    cfg.Bootstrap,
		Logger:       logger,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize sqlite-p2p engine: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	app := &ReplicationApp{
		cfg:      cfg,
		engine:   engine,
		logger:   logger,
		registry: NewCommandRegistry(),
		stopChan: make(chan struct{}),
	}

	app.initCommands()

	routableDHT := ""
	assignedPort := 0
	if engine.Swarm() != nil {
		dhtAddr := engine.Swarm().DHTAddr()
		routableDHT = p2p.NormalizeDHTAddr(dhtAddr)
		assignedPort = engine.Swarm().Port()

		// Start background LAN discovery beacon so other nodes on this Wi-Fi/LAN auto-discover this node
		beaconMsg := p2p.BeaconMessage{
			Cluster: "sqlite-p2p",
			Topic:   cfg.SwarmTopic,
			DHTAddr: routableDHT,
			NodeID:  cfg.NodeID,
		}
		_ = p2p.StartBeaconBroadcaster(context.Background(), beaconMsg, p2p.DefaultBeaconPort, 2500*time.Millisecond)
	}

	fmt.Printf("=================================================================\n")
	fmt.Printf("  sqlite-p2p Pear P2P Node [%s] Online\n", cfg.NodeID)
	fmt.Printf("  Pear Protocol: Hyperswarm + HyperDHT + Noise SecretStream\n")
	fmt.Printf("  Swarm Topic:   %s\n", cfg.SwarmTopic)
	if routableDHT != "" {
		fmt.Printf("  DHT Endpoint:  %s (Port %d, auto-discoverable on LAN, zero port forward needed)\n", routableDHT, assignedPort)
	}
	fmt.Printf("  Database:      %s\n", cfg.DBPath)
	fmt.Printf("=================================================================\n\n")

	// Connect to configured initial direct peers if any
	for _, pAddr := range cfg.PeerAddrs {
		go app.connectToPeer(context.Background(), pAddr)
	}

	// If one-shot command passed, execute and exit
	if *cmdExec != "" {
		if err := app.registry.Execute(context.Background(), app, *cmdExec); err != nil {
			fmt.Fprintf(os.Stderr, "Command failed: %v\n", err)
			os.Exit(1)
		}
		// Brief pause to allow broadcast
		time.Sleep(300 * time.Millisecond)
		return
	}

	// Otherwise, enter interactive REPL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal. Exiting...")
		close(app.stopChan)
		cancel()
		os.Exit(0)
	}()

	app.runInteractive(ctx)
}

func (a *ReplicationApp) connectToPeer(ctx context.Context, addr string) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Printf("[P2P] Invalid peer address %s: %v\n> ", addr, err)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Printf("[P2P] Invalid peer port in %s: %v\n> ", addr, err)
		return
	}

	if a.engine.Swarm() == nil {
		fmt.Println("[P2P] Swarm manager not initialized\n> ")
		return
	}

	if err := a.engine.Swarm().ConnectDirect(host, port); err != nil {
		fmt.Printf("[P2P] Direct connect to %s failed: %v\n> ", addr, err)
		return
	}

	fmt.Printf("[P2P] Direct encrypted Pear connection established to: %s\n> ", addr)

	if a.cfg.AutoSync && a.engine.ReplicationEngine() != nil {
		go func() {
			time.Sleep(200 * time.Millisecond)
			count, err := a.engine.ReplicationEngine().SyncPeers(ctx)
			if err == nil && count > 0 {
				fmt.Printf("[P2P] Synced %d feed changesets to peer %s\n> ", count, addr)
			}
		}()
	}
}

func (a *ReplicationApp) initCommands() {
	cr := a.registry

	cr.Register(Command{
		Name:        "help",
		Usage:       "help",
		Description: "Show available commands",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			fmt.Println("\nAvailable Commands:")
			fmt.Println("  put <key> <data>       - Upsert record, record changeset & broadcast")
			fmt.Println("  get <key>              - Retrieve record from local SQLite store")
			fmt.Println("  del <key>              - Delete record & broadcast OpDelete changeset")
			fmt.Println("  list [prefix]          - List stored records in local SQLite store")
			fmt.Println("  connect <host:port>    - Establish direct encrypted Pear connection")
			fmt.Println("  sync                   - Replay and broadcast all changesets to peers")
			fmt.Println("  peers                  - Display active peer connections")
			fmt.Println("  status                 - Display Pear Swarm & replication engine status")
			fmt.Println("  auto-test [count]      - Run automated batch write & replication test")
			fmt.Println("  help                   - Display this command reference")
			fmt.Println("  exit, quit             - Terminate node")
			fmt.Println("\nNote: Slash prefix syntax (e.g. /put, /get, /status) is also supported.")
			return nil
		},
	})

	cr.Register(Command{
		Name:        "put",
		Usage:       "put <key> <data>",
		Description: "Insert or update a record and broadcast changeset",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("usage: put <key> <data>")
			}
			key := args[0]
			data := strings.Join(args[1:], " ")
			meta := json.RawMessage(fmt.Sprintf(`{"node":%q,"updated_at":%d}`, app.cfg.NodeID, time.Now().Unix()))

			start := time.Now()
			err := app.engine.Put(ctx, key, meta, []byte(data))
			if err != nil {
				return fmt.Errorf("put failed: %w", err)
			}
			fmt.Printf("OK: Put %q (%d bytes) in %v. Changeset broadcasted to peers.\n", key, len(data), time.Since(start))
			return nil
		},
	})

	cr.Register(Command{
		Name:        "get",
		Usage:       "get <key>",
		Description: "Retrieve a record by key",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: get <key>")
			}
			key := args[0]
			rec, err := app.engine.Get(ctx, key)
			if err != nil {
				return fmt.Errorf("get %q: %w", key, err)
			}
			fmt.Printf("Record %q:\n", rec.Key)
			fmt.Printf("  Metadata: %s\n", string(rec.Metadata))
			fmt.Printf("  Data:     %s\n", string(rec.Data))
			return nil
		},
	})

	cr.Register(Command{
		Name:        "del",
		Usage:       "del <key>",
		Description: "Delete a record and broadcast OpDelete changeset",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: del <key>")
			}
			key := args[0]
			if err := app.engine.Delete(ctx, key); err != nil {
				return fmt.Errorf("delete %q: %w", key, err)
			}
			fmt.Printf("OK: Deleted %q. OpDelete broadcasted to peers.\n", key)
			return nil
		},
	})

	cr.Register(Command{
		Name:        "list",
		Usage:       "list [prefix]",
		Description: "List records in local store matching prefix",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			prefix := "%"
			if len(args) > 0 {
				prefix = args[0] + "%"
			}
			records, err := app.engine.List(ctx, prefix, 100, 0)
			if err != nil {
				return fmt.Errorf("list failed: %w", err)
			}
			if len(records) == 0 {
				fmt.Println("(No records found)")
				return nil
			}
			fmt.Printf("Found %d record(s):\n", len(records))
			for i, r := range records {
				preview := string(r.Data)
				if len(preview) > 60 {
					preview = preview[:57] + "..."
				}
				fmt.Printf("  [%d] Key: %-30s | Data: %s\n", i+1, r.Key, preview)
			}
			return nil
		},
	})

	cr.Register(Command{
		Name:        "connect",
		Usage:       "connect <host:port>",
		Description: "Connect directly to a peer using SecretStream encryption",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("usage: connect <host:port>")
			}
			app.connectToPeer(ctx, args[0])
			return nil
		},
	})

	cr.Register(Command{
		Name:        "sync",
		Usage:       "sync",
		Description: "Replay local feed changesets to all connected peers",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			if app.engine.ReplicationEngine() == nil {
				return fmt.Errorf("replication engine not active")
			}
			count, err := app.engine.ReplicationEngine().SyncPeers(ctx)
			if err != nil {
				return fmt.Errorf("sync failed: %w", err)
			}
			fmt.Printf("OK: Replayed and broadcasted %d changeset(s) to connected peers.\n", count)
			return nil
		},
	})

	cr.Register(Command{
		Name:        "peers",
		Usage:       "peers",
		Description: "Show connected peers",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			peerCount := 0
			if app.engine.Replicator() != nil {
				peerCount = app.engine.Replicator().PeerCount()
			}
			swarmPeers := 0
			if app.engine.Swarm() != nil {
				swarmPeers = app.engine.Swarm().PeerCount()
			}
			fmt.Printf("Active Peer Connections: %d (Swarm Mesh: %d)\n", peerCount, swarmPeers)
			return nil
		},
	})

	cr.Register(Command{
		Name:        "status",
		Usage:       "status",
		Description: "Show Pear Swarm and local node metrics",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			peerCount := 0
			if app.engine.Replicator() != nil {
				peerCount = app.engine.Replicator().PeerCount()
			}

			feedLen := uint64(0)
			if app.engine.ChangesetFeed() != nil {
				feedLen = app.engine.ChangesetFeed().Len()
			}

			dhtAddr := "N/A"
			assignedPort := 0
			if app.engine.Swarm() != nil {
				dhtAddr = app.engine.Swarm().DHTAddr()
				assignedPort = app.engine.Swarm().Port()
			}

			fmt.Printf("Node Status:\n")
			fmt.Printf("  Node ID:         %s\n", app.cfg.NodeID)
			fmt.Printf("  Protocol:        Pear Hyperswarm (HyperDHT + Noise SecretStream)\n")
			fmt.Printf("  Cluster Topic:   %s\n", app.cfg.SwarmTopic)
			fmt.Printf("  DHT Endpoint:    %s (Dynamic port %d, no static port forward needed)\n", dhtAddr, assignedPort)
			fmt.Printf("  Database:        %s\n", app.cfg.DBPath)
			fmt.Printf("  Active Peers:    %d\n", peerCount)
			fmt.Printf("  Feed Changesets: %d\n", feedLen)
			return nil
		},
	})

	cr.Register(Command{
		Name:        "auto-test",
		Usage:       "auto-test [count]",
		Description: "Run automated batch write test",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			count := 5
			if len(args) > 0 {
				_, _ = fmt.Sscanf(args[0], "%d", &count)
			}
			if count < 1 {
				count = 1
			}

			fmt.Printf("Starting auto-test: writing %d records from node [%s]...\n", count, app.cfg.NodeID)
			start := time.Now()
			for i := 1; i <= count; i++ {
				key := fmt.Sprintf("test:%s:item-%d", app.cfg.NodeID, i)
				data := fmt.Sprintf("Auto-generated payload #%d created at %s", i, time.Now().Format(time.RFC3339Nano))
				meta := json.RawMessage(fmt.Sprintf(`{"batch":%d,"seq":%d}`, count, i))

				if err := app.engine.Put(ctx, key, meta, []byte(data)); err != nil {
					return fmt.Errorf("auto-test failed at item %d: %w", i, err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			dur := time.Since(start)
			fmt.Printf("Auto-test completed: %d records written and replicated in %v (avg %v/op)\n", count, dur, dur/time.Duration(count))
			return nil
		},
	})

	exitCmd := Command{
		Name:        "exit",
		Usage:       "exit",
		Description: "Exit the application",
		Run: func(ctx context.Context, app *ReplicationApp, args []string) error {
			fmt.Println("Shutting down...")
			os.Exit(0)
			return nil
		},
	}
	cr.Register(exitCmd)
	cr.Register(Command{
		Name:        "quit",
		Usage:       "quit",
		Description: "Exit the application",
		Run:         exitCmd.Run,
	})
}

func (a *ReplicationApp) runInteractive(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Type 'help' for commands, or 'exit' to quit.")
	fmt.Print("> ")

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if err := a.registry.Execute(ctx, a, line); err != nil {
				fmt.Printf("Error: %v\n", err)
			}
		}
		fmt.Print("> ")
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Scanner error: %v\n", err)
	}
}
