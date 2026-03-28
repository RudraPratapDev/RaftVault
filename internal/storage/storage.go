package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// LogEntry represents a single entry in the Raft log
type LogEntry struct {
	Term    int     `json:"term"`
	Index   int     `json:"index"`
	Command Command `json:"command"`
}

// Command represents a state machine command
type Command struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// PersistentState holds the Raft state that must be persisted to disk
type PersistentState struct {
	CurrentTerm int        `json:"current_term"`
	VotedFor    string     `json:"voted_for"`
	Log         []LogEntry `json:"log"`
}

// Storage handles persistence of Raft state to disk
type Storage struct {
	mu      sync.Mutex
	dataDir string
	state   PersistentState
}

// NewStorage creates a new Storage instance
func NewStorage(dataDir string) (*Storage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir %s: %w", dataDir, err)
	}
	s := &Storage{
		dataDir: dataDir,
	}
	return s, nil
}

func (s *Storage) stateFilePath() string {
	return filepath.Join(s.dataDir, "raft_state.json")
}

// SaveState atomically persists the Raft state to disk
func (s *Storage) SaveState(state PersistentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = state

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpPath := s.stateFilePath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	if err := os.Rename(tmpPath, s.stateFilePath()); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// LoadState reads the persisted Raft state from disk
func (s *Storage) LoadState() (PersistentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.stateFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			// No persisted state, return defaults
			return PersistentState{
				CurrentTerm: 0,
				VotedFor:    "",
				Log:         []LogEntry{},
			}, nil
		}
		return PersistentState{}, fmt.Errorf("failed to read state file: %w", err)
	}

	var state PersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return PersistentState{}, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	if state.Log == nil {
		state.Log = []LogEntry{}
	}

	s.state = state
	return state, nil
}
