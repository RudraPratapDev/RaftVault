package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"raft-kms/internal/api"
	"raft-kms/internal/chaos"
	"raft-kms/internal/config"
	"raft-kms/internal/kms"
	"raft-kms/internal/raft"
	"raft-kms/internal/storage"
)

func main() {
	configPath := flag.String("config", "", "Path to config file (JSON)")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: raft-kms --config <path-to-config.json>\n")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("=================================================")
	log.Printf("  RaftKMS Node: %s", cfg.NodeID)
	log.Printf("  Address:      %s", cfg.Address)
	log.Printf("  Peers:        %v", cfg.Peers)
	log.Printf("  Data Dir:     %s", cfg.DataDir)
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

	// Initialize event log
	eventLog := raft.NewEventLog(500)

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

	log.Printf("[MAIN] Shutting down node %s...", cfg.NodeID)
	raftNode.Stop()
	log.Printf("[MAIN] Node %s stopped.", cfg.NodeID)
}
