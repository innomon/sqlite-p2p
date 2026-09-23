package ontology

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sqlite-p2p/internal/store"
	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a directory for markdown files containing ontology frontmatter
// and persists parsed nodes and edges into SQLite.
type Watcher struct {
	dir     string
	repo    *store.Repository
	fs      *fsnotify.Watcher
	logger  *slog.Logger
	cancel  context.CancelFunc
	done    chan struct{}
	mu      sync.Mutex
	running bool
}

// NewWatcher initializes a filesystem watcher for the target directory.
func NewWatcher(dir string, repo *store.Repository) (*Watcher, error) {
	if repo == nil {
		return nil, errors.New("ontology: repository cannot be nil")
	}

	if dir == "" {
		dir = "records"
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create watcher directory %s: %w", dir, err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize fsnotify watcher: %w", err)
	}

	return &Watcher{
		dir:  dir,
		repo: repo,
		fs:   fsw,
		done: make(chan struct{}),
	}, nil
}

// SetLogger attaches a structured logger to the Watcher.
func (w *Watcher) SetLogger(l *slog.Logger) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logger = l
}

// IngestFile reads and parses a markdown file, upserting its nodes and edges into SQLite.
func (w *Watcher) IngestFile(ctx context.Context, path string) (*ParsedGraph, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	parsed, err := ParseMarkdown(content)
	if err != nil {
		if errors.Is(err, ErrNoFrontmatter) || errors.Is(err, ErrSchemaMismatch) {
			// Not an ontology file; skip quietly
			return nil, nil
		}
		return nil, fmt.Errorf("error parsing ontology markdown %s: %w", path, err)
	}

	for _, n := range parsed.Nodes {
		if err := w.repo.UpsertNode(ctx, n); err != nil {
			return nil, fmt.Errorf("failed to upsert node %s from %s: %w", n.ID, path, err)
		}
	}

	for _, e := range parsed.Edges {
		if err := w.repo.UpsertEdge(ctx, e); err != nil {
			return nil, fmt.Errorf("failed to upsert edge (%s -> %s) from %s: %w", e.Source, e.Target, path, err)
		}
	}

	w.mu.Lock()
	l := w.logger
	w.mu.Unlock()
	if l != nil {
		l.Info("ingested ontology markdown file",
			"event", "ontology_file_ingested",
			"path", path,
			"nodes", len(parsed.Nodes),
			"edges", len(parsed.Edges),
		)
	}

	return parsed, nil
}

// IngestDirectory scans the directory for all markdown files and ingests them.
func (w *Watcher) IngestDirectory(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory %s: %w", w.dir, err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".markdown" {
			continue
		}

		fullPath := filepath.Join(w.dir, name)
		parsed, err := w.IngestFile(ctx, fullPath)
		if err != nil {
			return count, err
		}
		if parsed != nil {
			count++
		}
	}

	return count, nil
}

// Start begins background directory monitoring for file creations and modifications.
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return nil
	}

	if err := w.fs.Add(w.dir); err != nil {
		return fmt.Errorf("failed to add directory %s to watcher: %w", w.dir, err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.running = true

	go w.eventLoop(watchCtx)
	return nil
}

func (w *Watcher) eventLoop(ctx context.Context) {
	defer close(w.done)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) {
				ext := strings.ToLower(filepath.Ext(event.Name))
				if ext == ".md" || ext == ".markdown" {
					// Brief sleep to avoid partial write race condition
					time.Sleep(20 * time.Millisecond)
					_, _ = w.IngestFile(ctx, event.Name)
				}
			}
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			w.mu.Lock()
			l := w.logger
			w.mu.Unlock()
			if l != nil {
				l.Error("watcher error", "err", err)
			}
		}
	}
}

// Stop terminates the watcher loop and cleans up fsnotify resources.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	if w.cancel != nil {
		w.cancel()
	}
	_ = w.fs.Close()
	w.mu.Unlock()

	// Wait for eventLoop to exit
	select {
	case <-w.done:
	case <-time.After(500 * time.Millisecond):
	}

	return nil
}
