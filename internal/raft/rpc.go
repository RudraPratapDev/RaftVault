package raft

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// sendRequestVote sends a RequestVote RPC to a peer
func sendRequestVote(peer string, args RequestVoteArgs) (*RequestVoteReply, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	url := fmt.Sprintf("http://%s/raft/requestVote", peer)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body error: %w", err)
	}

	var reply RequestVoteReply
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &reply, nil
}

// sendAppendEntries sends an AppendEntries RPC to a peer
func sendAppendEntries(peer string, args AppendEntriesArgs) (*AppendEntriesReply, error) {
	data, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal error: %w", err)
	}

	url := fmt.Sprintf("http://%s/raft/appendEntries", peer)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[RPC] AppendEntries to %s returned status %d", peer, resp.StatusCode)
	}

	var reply AppendEntriesReply
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &reply, nil
}
