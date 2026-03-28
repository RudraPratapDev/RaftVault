package raft

import "raft-kms/internal/storage"

// Role represents the current role of a Raft node
type Role int

const (
	Follower Role = iota
	Candidate
	Leader
)

func (r Role) String() string {
	switch r {
	case Follower:
		return "FOLLOWER"
	case Candidate:
		return "CANDIDATE"
	case Leader:
		return "LEADER"
	default:
		return "UNKNOWN"
	}
}

// RequestVoteArgs represents the RequestVote RPC arguments
type RequestVoteArgs struct {
	Term         int    `json:"term"`
	CandidateID  string `json:"candidateId"`
	LastLogIndex int    `json:"lastLogIndex"`
	LastLogTerm  int    `json:"lastLogTerm"`
}

// RequestVoteReply represents the RequestVote RPC response
type RequestVoteReply struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"voteGranted"`
}

// AppendEntriesArgs represents the AppendEntries RPC arguments
type AppendEntriesArgs struct {
	Term         int                `json:"term"`
	LeaderID     string             `json:"leaderId"`
	PrevLogIndex int                `json:"prevLogIndex"`
	PrevLogTerm  int                `json:"prevLogTerm"`
	Entries      []storage.LogEntry `json:"entries"`
	LeaderCommit int                `json:"leaderCommit"`
}

// AppendEntriesReply represents the AppendEntries RPC response
type AppendEntriesReply struct {
	Term    int  `json:"term"`
	Success bool `json:"success"`
}
