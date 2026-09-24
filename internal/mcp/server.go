package mcp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"sqlite-p2p/internal/crypto"
	"sqlite-p2p/internal/p2p"
	"sqlite-p2p/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerOptions encapsulates all runtime configuration and dependencies for the MCP server.
type ServerOptions struct {
	Version   string
	Transport string // "stdio" or "sse"
	Host      string
	Port      string
	Path      string
	DBPath    string
	EnableWAL bool

	Repo    *store.Repository
	Tracker *store.ChangesetTracker
	KeyReg  *crypto.KeyRegistry
	Engine  *p2p.ReplicationEngine
	Swarm   *p2p.SwarmManager

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Server provides the MCP runtime hosting CRM tools.
type Server struct {
	opts      ServerOptions
	mcpServer *mcp.Server
	dbClose   func() error
}

// NewServer initializes a new MCP server with the given options.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Transport == "" {
		opts.Transport = "stdio"
	}
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Port == "" {
		opts.Port = "8083"
	}
	if opts.Path == "" {
		opts.Path = "/mcp"
	}
	if opts.Version == "" {
		opts.Version = "0.1.0"
	}

	var dbClose func() error
	if opts.Repo == nil && opts.DBPath != "" {
		db, err := store.OpenDB(opts.DBPath, opts.EnableWAL)
		if err != nil {
			return nil, fmt.Errorf("failed to open database at %s: %w", opts.DBPath, err)
		}
		dbClose = db.Close
		if err := store.InitOntologySchema(db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to init ontology schema: %w", err)
		}
		keys, err := crypto.NewKeyRegistry(db)
		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to init key registry: %w", err)
		}
		opts.KeyReg = keys
		opts.Repo = store.NewRepository(db)
		opts.Repo.SetKeyRegistry(keys)
		opts.Tracker = store.NewChangesetTracker(opts.Repo)
	}

	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "crm-mcp",
		Version: opts.Version,
	}, nil)

	s := &Server{
		opts:      opts,
		mcpServer: mcpSrv,
		dbClose:   dbClose,
	}

	s.registerAllTools()

	return s, nil
}

// Run starts the MCP server using the configured transport.
func (s *Server) Run(ctx context.Context) error {
	defer func() {
		if s.dbClose != nil {
			_ = s.dbClose()
		}
	}()

	switch s.opts.Transport {
	case "stdio":
		stdioTransport := &mcp.StdioTransport{}
		return s.mcpServer.Run(ctx, stdioTransport)
	case "sse":
		return s.runSSEServer(ctx)
	default:
		return fmt.Errorf("unsupported transport: %s", s.opts.Transport)
	}
}

func (s *Server) runSSEServer(ctx context.Context) error {
	handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server { return s.mcpServer }, nil)
	mux := http.NewServeMux()
	mux.Handle(s.opts.Path, handler)

	address := net.JoinHostPort(s.opts.Host, s.opts.Port)
	httpServer := &http.Server{
		Addr:    address,
		Handler: mux,
	}

	errChan := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

func (s *Server) registerAllTools() {
	if s.opts.Repo != nil {
		RegisterSchemaTools(s.mcpServer, s.opts.Repo)
		RegisterCustomerTools(s.mcpServer, s.opts.Repo, s.opts.KeyReg, s.opts.Tracker)
		RegisterOntologyTools(s.mcpServer, s.opts.Repo)
		RegisterP2PTools(s.mcpServer, s.opts, s.opts.Engine, s.opts.Swarm)
	}
}
