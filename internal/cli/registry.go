package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// HandlerFunc defines the execution signature for a CLI command.
type HandlerFunc func(ctx context.Context, args []string) error

// Command represents a handcrafted CLI command or subcommand.
type Command struct {
	Name        string
	Description string
	Usage       string
	Subcommands map[string]*Command
	Run         HandlerFunc
	Stdout      io.Writer
	Stderr      io.Writer
}

// NewCommand creates a new root or subcommand container.
func NewCommand(name, description string) *Command {
	return &Command{
		Name:        name,
		Description: description,
		Subcommands: make(map[string]*Command),
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}
}

// AddSubcommand registers a child subcommand.
func (c *Command) AddSubcommand(sub *Command) {
	if c.Subcommands == nil {
		c.Subcommands = make(map[string]*Command)
	}
	if sub.Stdout == nil {
		sub.Stdout = c.Stdout
	}
	if sub.Stderr == nil {
		sub.Stderr = c.Stderr
	}
	c.Subcommands[sub.Name] = sub
}

// Help generates a formatted usage help string.
func (c *Command) Help() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s - %s\n\n", c.Name, c.Description))

	if c.Usage != "" {
		b.WriteString(fmt.Sprintf("Usage: %s\n\n", c.Usage))
	} else if len(c.Subcommands) > 0 {
		b.WriteString(fmt.Sprintf("Usage: %s <subcommand> [flags] [args]\n\n", c.Name))
	}

	if len(c.Subcommands) > 0 {
		b.WriteString("Available Commands:\n")
		var names []string
		for name := range c.Subcommands {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			sub := c.Subcommands[name]
			b.WriteString(fmt.Sprintf("  %-14s %s\n", name, sub.Description))
		}
		b.WriteString("\nUse \"[command] --help\" for more information about a command.\n")
	}

	return b.String()
}

// Dispatch executes the command or traverses nested subcommands.
func (c *Command) Dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		if c.Run != nil {
			return c.Run(ctx, args)
		}
		fmt.Fprint(c.Stdout, c.Help())
		return nil
	}

	first := args[0]
	if first == "-h" || first == "--help" || first == "help" {
		fmt.Fprint(c.Stdout, c.Help())
		return nil
	}

	if sub, exists := c.Subcommands[first]; exists {
		return sub.Dispatch(ctx, args[1:])
	}

	if c.Run != nil {
		return c.Run(ctx, args)
	}

	return fmt.Errorf("unknown command: %s for %s", first, c.Name)
}
