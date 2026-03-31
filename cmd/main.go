package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"raft-kms/internal/api"
	"raft-kms/internal/chaos"
	"raft-kms/internal/config"
	"raft-kms/internal/kms"
	"raft-kms/internal/raft"
	"raft-kms/internal/storage"
)

func setupLogger(nodeID string, logDir string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}
	logPath := filepath.Join(logDir, nodeID+".log")
	// Open in append mode so logs persist across restarts
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	// Tee to both file and stdout
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	return f, nil
}

func main() {
	configPath := flag.String("config", "", "Path to config file (JSON)")
	logDir := flag.String("log-dir", "./logs", "Directory for persistent log files")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: raft-kms --config <path-to-config.json> [--log-dir ./logs]\n")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup persistent logger (append mode — survives restarts)
	logFile, err := setupLogger(cfg.NodeID, *logDir)
	if err != nil {
		log.Printf("[WARN] Could not open log file, logging to stdout only: %v", err)
	} else {
		defer logFile.Close()
	}

	log.Printf("=================================================")
	log.Printf("  RaftKMS Node: %s  [RESTART/START]", cfg.NodeID)
	log.Printf("  Address:      %s", cfg.Address)
	log.Printf("  Peers:        %v", cfg.Peers)
	log.Printf("  Data Dir:     %s", cfg.DataDir)
	log.Printf("  Log File:     %s/%s.log", *logDir, cfg.NodeID)
	log.Printf("  Started At:   %s", time.Now().Format(time.RFC3339))
	log.Printf("=================================================")

	// Initialize storage
	store, err := storage.NewStorage(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	// Initialize KMS store
	kmsStore := kms.NewKMSStore()

	// Initialize Raft node
	raftNode := raft.NewRaftNode(
		cfg.NodeID,
		cfg.Address,
		cfg.Peers,
		store,
		time.Duration(cfg.ElectionTimeoutMinMs)*time.Millisecond,
		time.Duration(cfg.ElectionTimeoutMaxMs)*time.Millisecond,
		time.Duration(cfg.HeartbeatIntervalMs)*time.Millisecond,
	)

	// Initialize chaos module
	chaosModule := chaos.NewChaosModule()

	// Initialize event log (larger buffer for demo)
	eventLog := raft.NewEventLog(1000)

	// Connect KMS as the state machine
	raftNode.SetApplyFunc(kmsStore.Apply)

	// Connect chaos module so killed nodes stop sending RPCs
	raftNode.SetKilledFunc(chaosModule.IsKilled)

	// Connect event log for dashboard streaming
	raftNode.SetEventLog(eventLog)

	// Start Raft
	if err := raftNode.Start(); err != nil {
		log.Fatalf("Failed to start Raft: %v", err)
	}

	// Start API server
	server := api.NewServer(cfg.Address, raftNode, kmsStore, chaosModule, eventLog)

	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	log.Printf("[MAIN] Node %s is running. Press Ctrl+C to stop.", cfg.NodeID)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("[MAIN] Shutting down node %s at %s...", cfg.NodeID, time.Now().Format(time.RFC3339))
	raftNode.Stop()
	log.Printf("[MAIN] Node %s stopped cleanly.", cfg.NodeID)
}
