package mcp

import (
	"context"
	"fmt"

	"crm-sqlite-pear-p2p/internal/p2p"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// P2PToolHandler provides MCP tooling for P2P swarm and replication monitoring.
type P2PToolHandler struct {
	opts   ServerOptions
	engine *p2p.ReplicationEngine
	swarm  *p2p.SwarmManager
}

// NewP2PToolHandler constructs a new P2PToolHandler.
func NewP2PToolHandler(opts ServerOptions, engine *p2p.ReplicationEngine, swarm *p2p.SwarmManager) *P2PToolHandler {
	return &P2PToolHandler{
		opts:   opts,
		engine: engine,
		swarm:  swarm,
	}
}

// RegisterP2PTools registers P2P observability tools with the MCP server.
func RegisterP2PTools(server *mcp.Server, opts ServerOptions, engine *p2p.ReplicationEngine, swarm *p2p.SwarmManager) {
	if server == nil {
		return
	}
	h := NewP2PToolHandler(opts, engine, swarm)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_p2p_status",
		Description: "Inspect P2P swarm status, local database path, WAL configuration, and peer connectivity.",
	}, h.P2PStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_p2p_peers",
		Description: "List active connected peers in the Hyperswarm cluster mesh.",
	}, h.P2PPeers)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "crm_p2p_sync",
		Description: "Trigger an immediate Autobase peer reconciliation sweep across connected peers.",
	}, h.P2PSync)
}

// --- P2P Status Tool ---

type P2PStatusInput struct{}

type P2PStatusOutput struct {
	Version        string `json:"version"`
	DBPath         string `json:"db_path"`
	WALMode        bool   `json:"wal_mode"`
	ConnectedPeers int    `json:"connected_peers"`
	ActiveTopics   int    `json:"active_topics"`
}

func (h *P2PToolHandler) P2PStatus(ctx context.Context, req *mcp.CallToolRequest, input P2PStatusInput) (*mcp.CallToolResult, P2PStatusOutput, error) {
	peerCount := 0
	activeTopics := 0
	if h.swarm != nil {
		stats := h.swarm.Stats()
		peerCount = stats.PeerCount
		activeTopics = stats.ActiveTopics
	}

	msg := fmt.Sprintf("Node Version: %s | DB: %s (WAL: %v) | Peers: %d | Topics: %d",
		h.opts.Version, h.opts.DBPath, h.opts.EnableWAL, peerCount, activeTopics)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, P2PStatusOutput{
		Version:        h.opts.Version,
		DBPath:         h.opts.DBPath,
		WALMode:        h.opts.EnableWAL,
		ConnectedPeers: peerCount,
		ActiveTopics:   activeTopics,
	}, nil
}

// --- P2P Peers Tool ---

type P2PPeersInput struct{}

type PeerInfo struct {
	Index int `json:"index"`
}

type P2PPeersOutput struct {
	Count int        `json:"count"`
	Peers []PeerInfo `json:"peers"`
}

func (h *P2PToolHandler) P2PPeers(ctx context.Context, req *mcp.CallToolRequest, input P2PPeersInput) (*mcp.CallToolResult, P2PPeersOutput, error) {
	count := 0
	if h.swarm != nil {
		count = h.swarm.PeerCount()
	}

	peers := make([]PeerInfo, count)
	for i := 0; i < count; i++ {
		peers[i] = PeerInfo{Index: i + 1}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Connected peers: %d", count)}},
	}, P2PPeersOutput{
		Count: count,
		Peers: peers,
	}, nil
}

// --- P2P Sync Tool ---

type P2PSyncInput struct{}

type P2PSyncOutput struct {
	BroadcastCount int    `json:"broadcast_count"`
	Message        string `json:"message"`
}

func (h *P2PToolHandler) P2PSync(ctx context.Context, req *mcp.CallToolRequest, input P2PSyncInput) (*mcp.CallToolResult, P2PSyncOutput, error) {
	count := 0
	if h.engine != nil {
		var err error
		count, err = h.engine.SyncPeers(ctx)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("p2p sync failed: %v", err)}},
			}, P2PSyncOutput{}, nil
		}
	}

	msg := fmt.Sprintf("Sync complete: %d changesets broadcasted to connected peers", count)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, P2PSyncOutput{
		BroadcastCount: count,
		Message:        msg,
	}, nil
}
