package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"sqlite-p2p/internal/cli"
	"sqlite-p2p/internal/config"
)

func TestBuildRootCommand(t *testing.T) {
	cfg := config.DefaultConfig()
	root := cli.BuildRootCommand("0.1.0", cfg)

	var out bytes.Buffer
	root.Stdout = &out

	// Test version subcommand
	err := root.Dispatch(context.Background(), []string{"version"})
	if err != nil {
		t.Fatalf("version dispatch failed: %v", err)
	}
	if !strings.Contains(out.String(), "crm-peer v0.1.0") {
		t.Errorf("expected version output, got: %s", out.String())
	}

	// Test status subcommand
	out.Reset()
	err = root.Dispatch(context.Background(), []string{"status"})
	if err != nil {
		t.Fatalf("status dispatch failed: %v", err)
	}
	if !strings.Contains(out.String(), "Node Status") {
		t.Errorf("expected status output to contain 'Node Status', got: %s", out.String())
	}
	if !strings.Contains(out.String(), cfg.DBPath) {
		t.Errorf("expected status output to contain DBPath, got: %s", out.String())
	}
}

func TestBuildRootCommandFlags(t *testing.T) {
	cfg := config.DefaultConfig()
	root := cli.BuildRootCommand("0.1.0", cfg)

	var out bytes.Buffer
	root.Stdout = &out

	// Test -v / --version
	err := root.Dispatch(context.Background(), []string{"--version"})
	if err != nil {
		t.Fatalf("--version dispatch failed: %v", err)
	}
	if !strings.Contains(out.String(), "0.1.0") {
		t.Errorf("expected version output, got: %s", out.String())
	}
}
