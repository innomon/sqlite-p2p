package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"crm-sqlite-pear-p2p/internal/cli"
)

func TestCommandRegistrationAndDispatch(t *testing.T) {
	root := cli.NewCommand("crm-peer", "Distributed Multimodal Agentic CRM CLI")

	var executed bool
	var capturedArgs []string

	statusCmd := &cli.Command{
		Name:        "status",
		Description: "Show node status",
		Usage:       "crm-peer status",
		Run: func(ctx context.Context, args []string) error {
			executed = true
			capturedArgs = args
			return nil
		},
	}

	root.AddSubcommand(statusCmd)

	err := root.Dispatch(context.Background(), []string{"status", "--verbose"})
	if err != nil {
		t.Fatalf("unexpected error dispatching status: %v", err)
	}

	if !executed {
		t.Errorf("expected command to execute")
	}

	if len(capturedArgs) != 1 || capturedArgs[0] != "--verbose" {
		t.Errorf("expected arg --verbose, got %v", capturedArgs)
	}
}

func TestNestedSubcommandDispatch(t *testing.T) {
	root := cli.NewCommand("crm-peer", "CRM CLI")
	customerCmd := &cli.Command{
		Name:        "customer",
		Description: "Customer record operations",
	}

	var subExecuted bool
	addCmd := &cli.Command{
		Name:        "add",
		Description: "Add customer",
		Run: func(ctx context.Context, args []string) error {
			subExecuted = true
			if len(args) == 0 || args[0] != "9876543210" {
				return errors.New("missing phone")
			}
			return nil
		},
	}

	customerCmd.AddSubcommand(addCmd)
	root.AddSubcommand(customerCmd)

	err := root.Dispatch(context.Background(), []string{"customer", "add", "9876543210"})
	if err != nil {
		t.Fatalf("unexpected error on nested dispatch: %v", err)
	}
	if !subExecuted {
		t.Errorf("expected nested subcommand to execute")
	}
}

func TestUnknownCommand(t *testing.T) {
	root := cli.NewCommand("crm-peer", "CRM CLI")
	err := root.Dispatch(context.Background(), []string{"unknown"})
	if err == nil {
		t.Fatalf("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "unknown command: unknown") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHelpDisplay(t *testing.T) {
	root := cli.NewCommand("crm-peer", "CRM Root Command")
	statusCmd := &cli.Command{
		Name:        "status",
		Description: "Display node and storage status",
	}
	root.AddSubcommand(statusCmd)

	helpText := root.Help()
	if !strings.Contains(helpText, "crm-peer") {
		t.Errorf("expected help text to contain command name, got: %s", helpText)
	}
	if !strings.Contains(helpText, "status") {
		t.Errorf("expected help text to list subcommand status, got: %s", helpText)
	}

	// Dispatching "help" or "--help"
	err := root.Dispatch(context.Background(), []string{"--help"})
	if err != nil {
		t.Errorf("dispatching --help should not error, got: %v", err)
	}
}
