package raft

import (
	"sync"
	"time"
)

// EventType represents the type of Raft event
type EventType string

const (
	EventElectionStart  EventType = "ELECTION_START"
	EventVoteGranted    EventType = "VOTE_GRANTED"
	EventVoteRejected   EventType = "VOTE_REJECTED"
	EventLeaderElected  EventType = "LEADER_ELECTED"
	EventHeartbeat      EventType = "HEARTBEAT"
	EventLogReplicated  EventType = "LOG_REPLICATED"
	EventEntryCommitted EventType = "ENTRY_COMMITTED"
	EventEntryApplied   EventType = "ENTRY_APPLIED"
	EventRoleChange     EventType = "ROLE_CHANGE"
	EventChaosKill      EventType = "CHAOS_KILL"
	EventChaosRevive    EventType = "CHAOS_REVIVE"
	EventChaosDelay     EventType = "CHAOS_DELAY"
	EventChaosDrop      EventType = "CHAOS_DROP"
	EventKeyCreated     EventType = "KEY_CREATED"
	EventKeyDeleted     EventType = "KEY_DELETED"
	EventKeyRotated     EventType = "KEY_ROTATED"
	EventEncrypt        EventType = "ENCRYPT"
	EventDecrypt        EventType = "DECRYPT"
	EventKMSCommand     EventType = "KMS_COMMAND"
	EventAppendEntries  EventType = "APPEND_ENTRIES"
	EventRequestVote    EventType = "REQUEST_VOTE"
)

// Event represents a single system event
type Event struct {
	ID        int                    `json:"id"`
	Timestamp string                 `json:"timestamp"`
	Type      EventType              `json:"type"`
	NodeID    string                 `json:"node_id"`
	Term      int                    `json:"term"`
	Details   map[string]interface{} `json:"details"`
}

// EventLog is a thread-safe ring buffer of events with subscriber support
type EventLog struct {
	mu          sync.RWMutex
	events      []Event
	maxSize     int
	nextID      int
	subscribers map[int]chan Event
	subIDCounter int
}

// NewEventLog creates a new EventLog with the given capacity
func NewEventLog(maxSize int) *EventLog {
	return &EventLog{
		events:      make([]Event, 0, maxSize),
		maxSize:     maxSize,
		nextID:      1,
		subscribers: make(map[int]chan Event),
	}
}

// Add adds a new event to the log and notifies all subscribers
func (el *EventLog) Add(eventType EventType, nodeID string, term int, details map[string]interface{}) {
	el.mu.Lock()

	event := Event{
		ID:        el.nextID,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Type:      eventType,
		NodeID:    nodeID,
		Term:      term,
		Details:   details,
	}
	el.nextID++

	if len(el.events) >= el.maxSize {
		el.events = el.events[1:]
	}
	el.events = append(el.events, event)

	// Copy subscribers to avoid holding lock during send
	subs := make(map[int]chan Event, len(el.subscribers))
	for id, ch := range el.subscribers {
		subs[id] = ch
	}
	el.mu.Unlock()

	// Notify subscribers (non-blocking)
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Subscriber is slow, skip
		}
	}
}

// GetAll returns all events
func (el *EventLog) GetAll() []Event {
	el.mu.RLock()
	defer el.mu.RUnlock()
	result := make([]Event, len(el.events))
	copy(result, el.events)
	return result
}

// GetSince returns events with ID > sinceID
func (el *EventLog) GetSince(sinceID int) []Event {
	el.mu.RLock()
	defer el.mu.RUnlock()

	var result []Event
	for _, e := range el.events {
		if e.ID > sinceID {
			result = append(result, e)
		}
	}
	return result
}

// Subscribe returns a channel that receives new events, and a subscription ID for unsubscribing
func (el *EventLog) Subscribe() (int, chan Event) {
	el.mu.Lock()
	defer el.mu.Unlock()

	el.subIDCounter++
	id := el.subIDCounter
	ch := make(chan Event, 100)
	el.subscribers[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber
func (el *EventLog) Unsubscribe(id int) {
	el.mu.Lock()
	defer el.mu.Unlock()

	if ch, ok := el.subscribers[id]; ok {
		close(ch)
		delete(el.subscribers, id)
	}
}
