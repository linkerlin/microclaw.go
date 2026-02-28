// Package runtime manages the microclaw runtime: starts all enabled channels.
package runtime

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/linkerlin/microclaw.go/internal/agent"
	"github.com/linkerlin/microclaw.go/internal/channels/telegram"
	"github.com/linkerlin/microclaw.go/internal/channels/web"
	"github.com/linkerlin/microclaw.go/internal/config"
	"github.com/linkerlin/microclaw.go/internal/db"
	"github.com/linkerlin/microclaw.go/internal/memory"
)

// Runtime manages all components of a running microclaw instance.
type Runtime struct {
	cfg    *config.Config
	db     *db.DB
	mem    *memory.Manager
	engine *agent.Engine
}

// New creates and initializes a Runtime from config.
func New(cfg *config.Config) (*Runtime, error) {
	// Ensure data directory exists.
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data dir: %w", err)
	}
	if err := os.MkdirAll(cfg.WorkingDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating working dir: %w", err)
	}

	// Open database.
	dbPath := filepath.Join(cfg.DataDir, "microclaw.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Create memory manager.
	mem := memory.New(cfg.DataDir)

	// Create agent engine.
	eng, err := agent.New(cfg, database, mem)
	if err != nil {
		return nil, fmt.Errorf("creating agent engine: %w", err)
	}

	return &Runtime{
		cfg:    cfg,
		db:     database,
		mem:    mem,
		engine: eng,
	}, nil
}

// Run starts all enabled channels and waits for shutdown.
func (r *Runtime) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle OS signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutdown signal received")
		cancel()
	}()

	var wg sync.WaitGroup
	errs := make(chan error, 4)

	// Start web channel.
	if r.cfg.Channels.Web.Enabled {
		webCh := web.New(r.cfg, r.db, r.mem, r.engine)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := webCh.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("Web channel error: %v", err)
				errs <- err
			}
		}()
	}

	// Start Telegram channel.
	if r.cfg.Channels.Telegram.Enabled {
		tgCh, err := telegram.New(r.cfg, r.db, r.mem, r.engine)
		if err != nil {
			log.Printf("Failed to start Telegram channel: %v", err)
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := tgCh.Run(ctx); err != nil && ctx.Err() == nil {
					log.Printf("Telegram channel error: %v", err)
					errs <- err
				}
			}()
		}
	}

	// Wait for all channels or context cancellation.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case err := <-errs:
		cancel()
		return err
	case <-ctx.Done():
		wg.Wait()
		return nil
	}
}

// DB returns the database instance (for CLI tools like password setting).
func (r *Runtime) DB() *db.DB {
	return r.db
}
