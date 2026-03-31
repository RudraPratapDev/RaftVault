package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"raft-kms/internal/storage"
	"sync"
	"time"
)

// ApplyFunc is called when a committed log entry should be applied to the state machine
type ApplyFunc func(command storage.Command) (interface{}, error)

// RaftNode represents a single node in the Raft cluster
type RaftNode struct {
	mu sync.RWMutex

	// Identity
	id      string
	address string
	peers   []string

	// Persistent state (saved to disk)
	currentTerm int
	votedFor    string
	log         []storage.LogEntry

	// Volatile state
	commitIndex int
	lastApplied int
	role        Role
	leaderID    string

	// Leader-only volatile state
	nextIndex  map[string]int
	matchIndex map[string]int

	// Timing
	electionTimeoutMin time.Duration
	electionTimeoutMax time.Duration
	heartbeatInterval  time.Duration
	lastHeartbeat      time.Time

	// Components
	store    *storage.Storage
	applyFn  ApplyFunc
	killedFn func() bool
	events   *EventLog

	// Channels
	stopCh    chan struct{}
	applyCh   chan applyResult
	commitCh  chan struct{}

	// Pending client requests waiting for commit
	pendingMu     sync.Mutex
	pendingApply  map[int]chan applyResult
}

type applyResult struct {
	Result interface{}
	Err    error
}

// NewRaftNode creates a new Raft node
func NewRaftNode(id string, address string, peers []string, store *storage.Storage,
	electionMin, electionMax, heartbeat time.Duration) *RaftNode {

	return &RaftNode{
		id:                 id,
		address:            address,
		peers:              peers,
		role:               Follower,
		currentTerm:        0,
		votedFor:           "",
		log:                []storage.LogEntry{},
		commitIndex:        0,
		lastApplied:        0,
		leaderID:           "",
		nextIndex:          make(map[string]int),
		matchIndex:         make(map[string]int),
		electionTimeoutMin: electionMin,
		electionTimeoutMax: electionMax,
		heartbeatInterval:  heartbeat,
		lastHeartbeat:      time.Now(),
		store:              store,
		stopCh:             make(chan struct{}),
		commitCh:           make(chan struct{}, 100),
		pendingApply:       make(map[int]chan applyResult),
	}
}

// SetApplyFunc sets the function called when entries are applied to the state machine
func (rn *RaftNode) SetApplyFunc(fn ApplyFunc) {
	rn.applyFn = fn
}

// SetKilledFunc sets a function that returns true when the node should act as if killed
func (rn *RaftNode) SetKilledFunc(fn func() bool) {
	rn.killedFn = fn
}

// SetEventLog sets the event log for this node
func (rn *RaftNode) SetEventLog(el *EventLog) {
	rn.events = el
}

// GetEventLog returns the event log
func (rn *RaftNode) GetEventLog() *EventLog {
	return rn.events
}

// emitEvent adds an event to the log if available
func (rn *RaftNode) emitEvent(eventType EventType, term int, details map[string]interface{}) {
	if rn.events != nil {
		rn.events.Add(eventType, rn.id, term, details)
	}
}

// isKilled checks if the node is in a killed state
func (rn *RaftNode) isKilled() bool {
	if rn.killedFn != nil {
		return rn.killedFn()
	}
	return false
}

// Start initializes the Raft node and begins election timer
func (rn *RaftNode) Start() error {
	// Load persisted state
	state, err := rn.store.LoadState()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	rn.mu.Lock()
	rn.currentTerm = state.CurrentTerm
	rn.votedFor = state.VotedFor
	rn.log = state.Log
	if rn.log == nil {
		rn.log = []storage.LogEntry{}
	}
	rn.mu.Unlock()

	log.Printf("[RAFT] Node %s started | Term=%d | VotedFor=%q | LogLen=%d | Recovering committed entries...",
		rn.id, rn.currentTerm, rn.votedFor, len(rn.log))

	// Re-apply committed entries to rebuild state machine
	rn.mu.Lock()
	for i := 0; i < len(rn.log) && i < rn.commitIndex; i++ {
		if rn.applyFn != nil {
			rn.applyFn(rn.log[i].Command)
		}
	}
	rn.lastApplied = rn.commitIndex
	rn.mu.Unlock()

	go rn.electionTimerLoop()
	go rn.applyLoop()

	return nil
}

// Stop gracefully stops the Raft node
func (rn *RaftNode) Stop() {
	close(rn.stopCh)
}

// GetID returns the node ID
func (rn *RaftNode) GetID() string {
	return rn.id
}

// GetRole returns the current role
func (rn *RaftNode) GetRole() Role {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	return rn.role
}

// GetLeaderID returns the current leader's ID
func (rn *RaftNode) GetLeaderID() string {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	return rn.leaderID
}

// GetState returns the current node state for status reporting
func (rn *RaftNode) GetState() map[string]interface{} {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	return map[string]interface{}{
		"node_id":      rn.id,
		"role":         rn.role.String(),
		"current_term": rn.currentTerm,
		"leader_id":    rn.leaderID,
		"commit_index": rn.commitIndex,
		"last_applied": rn.lastApplied,
		"log_length":   len(rn.log),
		"peers":        rn.peers,
	}
}

// GetLeaderAddress returns the leader's address for redirects
func (rn *RaftNode) GetLeaderAddress() string {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	leaderID := rn.leaderID
	if leaderID == rn.id {
		return ""
	}

	// The leaderID is stored as the peer address
	return leaderID
}

// SubmitCommand submits a command to the Raft log (leader only)
func (rn *RaftNode) SubmitCommand(command storage.Command) (interface{}, error) {
	rn.mu.Lock()
	if rn.role != Leader {
		rn.mu.Unlock()
		return nil, fmt.Errorf("not leader")
	}

	// Create log entry
	lastIndex := len(rn.log)
	entry := storage.LogEntry{
		Term:    rn.currentTerm,
		Index:   lastIndex + 1,
		Command: command,
	}

	rn.log = append(rn.log, entry)
	rn.persist()

	log.Printf("[RAFT] Leader %s: Appended entry Index=%d Term=%d Action=%s",
		rn.id, entry.Index, entry.Term, command.Action)

	// Create a channel to wait for the commit
	resultCh := make(chan applyResult, 1)
	rn.pendingMu.Lock()
	rn.pendingApply[entry.Index] = resultCh
	rn.pendingMu.Unlock()

	rn.mu.Unlock()

	// Trigger immediate replication
	go rn.replicateToAll()

	// Wait for commit with timeout
	select {
	case result := <-resultCh:
		return result.Result, result.Err
	case <-time.After(10 * time.Second):
		rn.pendingMu.Lock()
		delete(rn.pendingApply, entry.Index)
		rn.pendingMu.Unlock()
		return nil, fmt.Errorf("commit timeout")
	case <-rn.stopCh:
		return nil, fmt.Errorf("node stopped")
	}
}

// persist saves the current state to disk
func (rn *RaftNode) persist() {
	state := storage.PersistentState{
		CurrentTerm: rn.currentTerm,
		VotedFor:    rn.votedFor,
		Log:         rn.log,
	}
	if err := rn.store.SaveState(state); err != nil {
		log.Printf("[RAFT] ERROR: Failed to persist state: %v", err)
	}
}

// randomElectionTimeout returns a random election timeout duration
func (rn *RaftNode) randomElectionTimeout() time.Duration {
	diff := rn.electionTimeoutMax - rn.electionTimeoutMin
	return rn.electionTimeoutMin + time.Duration(rand.Int63n(int64(diff)))
}

// electionTimerLoop runs the election timer
func (rn *RaftNode) electionTimerLoop() {
	for {
		timeout := rn.randomElectionTimeout()
		timer := time.NewTimer(timeout)

		select {
		case <-timer.C:
			if rn.isKilled() {
				continue
			}
			rn.mu.Lock()
			if rn.role != Leader {
				elapsed := time.Since(rn.lastHeartbeat)
				if elapsed >= timeout {
					rn.mu.Unlock()
					rn.startElection()
				} else {
					rn.mu.Unlock()
				}
			} else {
				rn.mu.Unlock()
			}
		case <-rn.stopCh:
			timer.Stop()
			return
		}
	}
}

// startElection initiates a new leader election
func (rn *RaftNode) startElection() {
	if rn.isKilled() {
		return
	}
	rn.mu.Lock()

	rn.role = Candidate
	rn.currentTerm++
	rn.votedFor = rn.id
	rn.leaderID = ""
	rn.lastHeartbeat = time.Now()
	rn.persist()

	term := rn.currentTerm
	lastLogIndex := len(rn.log)
	lastLogTerm := 0
	if lastLogIndex > 0 {
		lastLogTerm = rn.log[lastLogIndex-1].Term
	}
	totalNodes := len(rn.peers) + 1
	majority := totalNodes/2 + 1

	log.Printf("[ELECTION] ⚡ Node %s timed out waiting for heartbeat — starting election | Term=%d | LogLen=%d | Need=%d/%d votes",
		rn.id, term, lastLogIndex, majority, totalNodes)
	rn.emitEvent(EventElectionStart, term, map[string]interface{}{
		"message":       "Election timeout — starting election",
		"need_votes":    majority,
		"total_nodes":   totalNodes,
		"last_log_index": lastLogIndex,
		"last_log_term": lastLogTerm,
	})

	rn.mu.Unlock()

	args := RequestVoteArgs{
		Term:         term,
		CandidateID:  rn.id,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}

	votesReceived := 1 // vote for self
	log.Printf("[ELECTION] Node %s: Voted for self (1/%d). Requesting votes from peers: %v", rn.id, majority, rn.peers)

	var votesMu sync.Mutex
	voteDone := make(chan struct{})

	for _, peer := range rn.peers {
		go func(p string) {
			log.Printf("[ELECTION] Node %s → %s: Sending RequestVote | Term=%d LastLogIndex=%d LastLogTerm=%d",
				rn.id, p, term, lastLogIndex, lastLogTerm)
			reply, err := sendRequestVote(p, args)
			if err != nil {
				log.Printf("[ELECTION] Node %s → %s: RequestVote FAILED (peer unreachable): %v", rn.id, p, err)
				rn.emitEvent(EventVoteRejected, term, map[string]interface{}{
					"from": p, "reason": "unreachable",
				})
				return
			}

			rn.mu.Lock()
			if reply.Term > rn.currentTerm {
				log.Printf("[ELECTION] Node %s: Saw higher term %d from %s — stepping down to FOLLOWER",
					rn.id, reply.Term, p)
				rn.currentTerm = reply.Term
				rn.role = Follower
				rn.votedFor = ""
				rn.leaderID = ""
				rn.persist()
				rn.mu.Unlock()
				return
			}
			rn.mu.Unlock()

			if reply.VoteGranted {
				votesMu.Lock()
				votesReceived++
				current := votesReceived
				votesMu.Unlock()
				log.Printf("[ELECTION] Node %s ← %s: Vote GRANTED ✓ | Votes=%d/%d (need %d)",
					rn.id, p, current, totalNodes, majority)
				rn.emitEvent(EventVoteGranted, term, map[string]interface{}{
					"from": p, "votes_so_far": current, "need": majority,
				})
				votesMu.Lock()
				if votesReceived >= majority {
					select {
					case voteDone <- struct{}{}:
					default:
					}
				}
				votesMu.Unlock()
			} else {
				log.Printf("[ELECTION] Node %s ← %s: Vote DENIED ✗ | peer's term=%d", rn.id, p, reply.Term)
				rn.emitEvent(EventVoteRejected, term, map[string]interface{}{
					"from": p, "peer_term": reply.Term,
				})
			}
		}(peer)
	}

	// Wait for majority or timeout
	select {
	case <-voteDone:
		rn.mu.Lock()
		if rn.currentTerm == term && rn.role == Candidate {
			log.Printf("[ELECTION] Node %s: Won election with majority votes! Becoming LEADER for Term %d", rn.id, term)
			rn.becomeLeader()
		}
		rn.mu.Unlock()
	case <-time.After(rn.randomElectionTimeout()):
		rn.mu.Lock()
		if rn.role == Candidate {
			log.Printf("[ELECTION] Node %s: Election TIMED OUT for Term %d (split vote or no quorum) — reverting to FOLLOWER", rn.id, term)
			rn.role = Follower
			rn.emitEvent(EventRoleChange, term, map[string]interface{}{
				"from": "CANDIDATE", "to": "FOLLOWER", "reason": "election timeout / split vote",
			})
		}
		rn.mu.Unlock()
	case <-rn.stopCh:
	}
}

// becomeLeader transitions this node to the leader role
func (rn *RaftNode) becomeLeader() {
	rn.role = Leader
	rn.leaderID = rn.id

	// Initialize leader state
	lastLogIndex := len(rn.log) + 1
	for _, peer := range rn.peers {
		rn.nextIndex[peer] = lastLogIndex
		rn.matchIndex[peer] = 0
	}

	log.Printf("[LEADER] 🎉 Node %s became LEADER for Term %d | LogLen=%d | Peers=%v",
		rn.id, rn.currentTerm, len(rn.log), rn.peers)
	rn.emitEvent(EventLeaderElected, rn.currentTerm, map[string]interface{}{
		"message":    "Became leader",
		"log_length": len(rn.log),
		"peers":      rn.peers,
	})
	rn.emitEvent(EventRoleChange, rn.currentTerm, map[string]interface{}{
		"from": "CANDIDATE", "to": "LEADER",
	})

	// Start heartbeat goroutine
	go rn.heartbeatLoop()
}

// heartbeatLoop periodically sends heartbeats to all peers
func (rn *RaftNode) heartbeatLoop() {
	ticker := time.NewTicker(rn.heartbeatInterval)
	defer ticker.Stop()

	// Send immediate heartbeat
	rn.replicateToAll()

	for {
		select {
		case <-ticker.C:
			if rn.isKilled() {
				continue
			}
			rn.mu.Lock()
			if rn.role != Leader {
				rn.mu.Unlock()
				return
			}
			rn.mu.Unlock()
			rn.replicateToAll()
		case <-rn.stopCh:
			return
		}
	}
}

// replicateToAll sends AppendEntries to all peers
func (rn *RaftNode) replicateToAll() {
	if rn.isKilled() {
		return
	}
	rn.mu.Lock()
	if rn.role != Leader {
		rn.mu.Unlock()
		return
	}
	peers := make([]string, len(rn.peers))
	copy(peers, rn.peers)
	rn.mu.Unlock()

	for _, peer := range peers {
		go rn.replicateTo(peer)
	}
}

// replicateTo sends AppendEntries to a specific peer
func (rn *RaftNode) replicateTo(peer string) {
	rn.mu.Lock()
	if rn.role != Leader {
		rn.mu.Unlock()
		return
	}

	nextIdx := rn.nextIndex[peer]
	prevLogIndex := nextIdx - 1
	prevLogTerm := 0
	if prevLogIndex > 0 && prevLogIndex <= len(rn.log) {
		prevLogTerm = rn.log[prevLogIndex-1].Term
	}

	// Get entries to send
	var entries []storage.LogEntry
	if nextIdx <= len(rn.log) {
		entries = make([]storage.LogEntry, len(rn.log)-nextIdx+1)
		copy(entries, rn.log[nextIdx-1:])
	}

	args := AppendEntriesArgs{
		Term:         rn.currentTerm,
		LeaderID:     rn.id,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: rn.commitIndex,
	}
	term := rn.currentTerm
	rn.mu.Unlock()

	reply, err := sendAppendEntries(peer, args)
	if err != nil {
		return
	}

	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.currentTerm != term || rn.role != Leader {
		return
	}

	if reply.Term > rn.currentTerm {
		rn.currentTerm = reply.Term
		rn.role = Follower
		rn.votedFor = ""
		rn.leaderID = ""
		rn.persist()
		log.Printf("[RAFT] Node %s: Stepping down, higher term %d from %s", rn.id, reply.Term, peer)
		return
	}

	if reply.Success {
		if len(entries) > 0 {
			rn.nextIndex[peer] = entries[len(entries)-1].Index + 1
			rn.matchIndex[peer] = entries[len(entries)-1].Index
			log.Printf("[REPLICATION] Leader %s → %s: Replicated %d entries | matchIndex=%d | nextIndex=%d",
				rn.id, peer, len(entries), rn.matchIndex[peer], rn.nextIndex[peer])
			rn.emitEvent(EventLogReplicated, rn.currentTerm, map[string]interface{}{
				"peer":        peer,
				"match_index": rn.matchIndex[peer],
				"entries":     len(entries),
			})
		}
		rn.advanceCommitIndex()
	} else {
		// Decrement nextIndex and retry
		if rn.nextIndex[peer] > 1 {
			rn.nextIndex[peer]--
			log.Printf("[REPLICATION] Leader %s → %s: Log inconsistency — backing nextIndex to %d for retry",
				rn.id, peer, rn.nextIndex[peer])
		}
	}
}

// advanceCommitIndex updates commitIndex based on majority replication
func (rn *RaftNode) advanceCommitIndex() {
	for n := len(rn.log); n > rn.commitIndex; n-- {
		if rn.log[n-1].Term != rn.currentTerm {
			continue
		}

		// Count replications
		replicatedCount := 1 // self
		for _, peer := range rn.peers {
			if rn.matchIndex[peer] >= n {
				replicatedCount++
			}
		}

		majority := (len(rn.peers) + 1) / 2 + 1
		if replicatedCount >= majority {
			log.Printf("[COMMIT] Leader %s: Entry Index=%d replicated on %d/%d nodes — COMMITTED ✓",
				rn.id, n, replicatedCount, len(rn.peers)+1)
			rn.emitEvent(EventEntryCommitted, rn.currentTerm, map[string]interface{}{
				"commit_index":      n,
				"replicated_on":     replicatedCount,
				"total_nodes":       len(rn.peers) + 1,
			})
			rn.commitIndex = n
			// Signal the apply loop
			select {
			case rn.commitCh <- struct{}{}:
			default:
			}
			break
		}
	}
}

// applyLoop applies committed entries to the state machine
func (rn *RaftNode) applyLoop() {
	for {
		select {
		case <-rn.commitCh:
			rn.applyCommitted()
		case <-rn.stopCh:
			return
		}
	}
}

// applyCommitted applies all committed but not yet applied entries
func (rn *RaftNode) applyCommitted() {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	for rn.lastApplied < rn.commitIndex {
		rn.lastApplied++
		entry := rn.log[rn.lastApplied-1]

		rn.emitEvent(EventEntryApplied, entry.Term, map[string]interface{}{
			"index": entry.Index, "action": entry.Command.Action,
		})
		log.Printf("[APPLY] Node %s: Applying entry Index=%d Term=%d Action=%s | lastApplied=%d commitIndex=%d",
			rn.id, entry.Index, entry.Term, entry.Command.Action, rn.lastApplied, rn.commitIndex)

		var result applyResult
		
		if entry.Command.Action == "ADD_NODE" {
			peerAddr := string(entry.Command.Payload)
			exists := false
			for _, p := range rn.peers {
				if p == peerAddr {
					exists = true
					break
				}
			}
			if !exists && peerAddr != rn.address {
				rn.peers = append(rn.peers, peerAddr)
				rn.nextIndex[peerAddr] = len(rn.log) + 1
				rn.matchIndex[peerAddr] = 0
				log.Printf("[RAFT] Node %s dynamically added peer %s. New total nodes: %d", rn.id, peerAddr, len(rn.peers)+1)
				rn.emitEvent(EventRoleChange, rn.currentTerm, map[string]interface{}{"message": "added peer " + peerAddr})
			}
			result = applyResult{Result: map[string]string{"message": "node added"}, Err: nil}
		} else if entry.Command.Action == "REMOVE_NODE" {
			peerAddr := string(entry.Command.Payload)
			newPeers := make([]string, 0, len(rn.peers))
			for _, p := range rn.peers {
				if p != peerAddr {
					newPeers = append(newPeers, p)
				}
			}
			rn.peers = newPeers
			delete(rn.nextIndex, peerAddr)
			delete(rn.matchIndex, peerAddr)
			log.Printf("[RAFT] Node %s dynamically removed peer %s. New total nodes: %d", rn.id, peerAddr, len(rn.peers)+1)
			rn.emitEvent(EventRoleChange, rn.currentTerm, map[string]interface{}{"message": "removed peer " + peerAddr})
			result = applyResult{Result: map[string]string{"message": "node removed"}, Err: nil}
		} else if rn.applyFn != nil {
			res, err := rn.applyFn(entry.Command)
			result = applyResult{Result: res, Err: err}
		}

		// Notify pending client request
		rn.pendingMu.Lock()
		if ch, ok := rn.pendingApply[entry.Index]; ok {
			ch <- result
			delete(rn.pendingApply, entry.Index)
		}
		rn.pendingMu.Unlock()
	}
}

// HandleRequestVote processes a RequestVote RPC
func (rn *RaftNode) HandleRequestVote(args RequestVoteArgs) RequestVoteReply {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	reply := RequestVoteReply{
		Term:        rn.currentTerm,
		VoteGranted: false,
	}

	// If candidate's term is older, reject
	if args.Term < rn.currentTerm {
		return reply
	}

	// If we see a higher term, update
	if args.Term > rn.currentTerm {
		rn.currentTerm = args.Term
		rn.role = Follower
		rn.votedFor = ""
		rn.leaderID = ""
		rn.persist()
	}

	reply.Term = rn.currentTerm

	// Check if we can vote for this candidate
	if rn.votedFor == "" || rn.votedFor == args.CandidateID {
		// Check if candidate's log is at least as up-to-date
		lastLogIndex := len(rn.log)
		lastLogTerm := 0
		if lastLogIndex > 0 {
			lastLogTerm = rn.log[lastLogIndex-1].Term
		}

		logOk := args.LastLogTerm > lastLogTerm ||
			(args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)

		if logOk {
			rn.votedFor = args.CandidateID
			rn.lastHeartbeat = time.Now()
			rn.persist()
			reply.VoteGranted = true
			log.Printf("[VOTE] Node %s: Granted vote to %s for Term %d | candidate log up-to-date ✓",
				rn.id, args.CandidateID, args.Term)
			rn.emitEvent(EventRequestVote, args.Term, map[string]interface{}{
				"candidate": args.CandidateID, "granted": true,
			})
		} else {
			log.Printf("[VOTE] Node %s: Denied vote to %s for Term %d | candidate log is stale (lastLogTerm=%d lastLogIndex=%d, mine=%d/%d)",
				rn.id, args.CandidateID, args.Term, args.LastLogTerm, args.LastLogIndex, lastLogTerm, lastLogIndex)
			rn.emitEvent(EventRequestVote, args.Term, map[string]interface{}{
				"candidate": args.CandidateID, "granted": false, "reason": "stale log",
			})
		}
	} else {
		log.Printf("[VOTE] Node %s: Denied vote to %s for Term %d | already voted for %s",
			rn.id, args.CandidateID, args.Term, rn.votedFor)
	}

	return reply
}

// HandleAppendEntries processes an AppendEntries RPC
func (rn *RaftNode) HandleAppendEntries(args AppendEntriesArgs) AppendEntriesReply {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	reply := AppendEntriesReply{
		Term:    rn.currentTerm,
		Success: false,
	}

	// Reject if term is older
	if args.Term < rn.currentTerm {
		return reply
	}

	// Update term if needed
	if args.Term > rn.currentTerm {
		rn.currentTerm = args.Term
		rn.votedFor = ""
		rn.persist()
	}

	// Recognize the leader
	if rn.role != Follower {
		rn.emitEvent(EventRoleChange, args.Term, map[string]interface{}{
			"from": rn.role.String(), "to": "FOLLOWER", "leader": args.LeaderID,
		})
	}
	rn.role = Follower
	rn.leaderID = args.LeaderID
	rn.lastHeartbeat = time.Now()

	reply.Term = rn.currentTerm

	// Check log consistency
	if args.PrevLogIndex > 0 {
		if args.PrevLogIndex > len(rn.log) {
			return reply
		}
		if rn.log[args.PrevLogIndex-1].Term != args.PrevLogTerm {
			// Delete conflicting entry and all that follow
			rn.log = rn.log[:args.PrevLogIndex-1]
			rn.persist()
			return reply
		}
	}

	// Append new entries
	if len(args.Entries) > 0 {
		// Find insertion point
		insertIdx := args.PrevLogIndex
		for i, entry := range args.Entries {
			idx := insertIdx + i
			if idx < len(rn.log) {
				if rn.log[idx].Term != entry.Term {
					// Conflict: delete this and all following
					log.Printf("[SYNC] Node %s: Log conflict at index %d (mine term=%d, leader term=%d) — truncating and rewriting",
						rn.id, idx+1, rn.log[idx].Term, entry.Term)
					rn.log = rn.log[:idx]
					rn.log = append(rn.log, args.Entries[i:]...)
					break
				}
			} else {
				rn.log = append(rn.log, args.Entries[i:]...)
				break
			}
		}
		rn.persist()
		log.Printf("[SYNC] Node %s ← %s: Appended %d entries | LogLen now=%d | commitIndex=%d",
			rn.id, args.LeaderID, len(args.Entries), len(rn.log), rn.commitIndex)
		rn.emitEvent(EventAppendEntries, args.Term, map[string]interface{}{
			"entries_count": len(args.Entries),
			"log_length":    len(rn.log),
			"from":          args.LeaderID,
		})
	}

	// Update commit index
	if args.LeaderCommit > rn.commitIndex {
		oldCommit := rn.commitIndex
		if args.LeaderCommit < len(rn.log) {
			rn.commitIndex = args.LeaderCommit
		} else {
			rn.commitIndex = len(rn.log)
		}
		if rn.commitIndex > oldCommit {
			select {
			case rn.commitCh <- struct{}{}:
			default:
			}
		}
	}

	reply.Success = true
	return reply
}

// IsLeader returns true if this node is the current leader
func (rn *RaftNode) IsLeader() bool {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	return rn.role == Leader
}

// GetPeers returns the list of peer addresses
func (rn *RaftNode) GetPeers() []string {
	return rn.peers
}

// GetLog returns a copy of the current log
func (rn *RaftNode) GetLog() []storage.LogEntry {
	rn.mu.Lock()
	defer rn.mu.Unlock()
	logCopy := make([]storage.LogEntry, len(rn.log))
	copy(logCopy, rn.log)
	return logCopy
}

// FormatLogForDisplay returns a human-readable representation of the log
func (rn *RaftNode) FormatLogForDisplay() []map[string]interface{} {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	result := make([]map[string]interface{}, len(rn.log))
	for i, entry := range rn.log {
		var payload interface{}
		json.Unmarshal(entry.Command.Payload, &payload)
		result[i] = map[string]interface{}{
			"index":   entry.Index,
			"term":    entry.Term,
			"action":  entry.Command.Action,
			"payload": payload,
		}
	}
	return result
}
