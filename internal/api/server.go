package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"raft-kms/internal/chaos"
	"raft-kms/internal/kms"
	"raft-kms/internal/raft"
	"raft-kms/internal/storage"
	"time"
)

type contextKey string
const userContextKey = contextKey("user")

// Server is the HTTP API server
type Server struct {
	raftNode *raft.RaftNode
	kmsStore *kms.KMSStore
	chaos    *chaos.ChaosModule
	address  string
	events   *raft.EventLog
}

// NewServer creates a new API server
func NewServer(address string, raftNode *raft.RaftNode, kmsStore *kms.KMSStore, chaosModule *chaos.ChaosModule, events *raft.EventLog) *Server {
	return &Server{
		raftNode: raftNode,
		kmsStore: kmsStore,
		chaos:    chaosModule,
		address:  address,
		events:   events,
	}
}

// requireAuth ensures the request has a valid API key
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")
		user, err := s.kmsStore.GetUserByAPIKey(apiKey)
		if err != nil {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin ensures the authenticated user is an admin
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(userContextKey).(*kms.User)
		if user.Role != kms.RoleAdmin {
			http.Error(w, `{"error":"forbidden: admin access required"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Raft RPC endpoints
	mux.HandleFunc("/raft/requestVote", s.chaosMiddleware(s.handleRequestVote))
	mux.HandleFunc("/raft/appendEntries", s.chaosMiddleware(s.handleAppendEntries))
	mux.HandleFunc("/raft/log", s.cors(s.handleRaftLog))

	// KMS endpoints (Admin)
	mux.HandleFunc("/kms/createKey", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleCreateKey))))
	mux.HandleFunc("/kms/deleteKey", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleDeleteKey))))
	mux.HandleFunc("/kms/rotateKey", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleRotateKey))))
	mux.HandleFunc("/kms/createUser", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleCreateUser))))
	mux.HandleFunc("/kms/deleteUser", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleDeleteUser))))

	// KMS endpoints (Authenticated users)
	mux.HandleFunc("/kms/getKey", s.cors(s.chaosMiddleware(s.requireAuth(s.handleGetKey))))
	mux.HandleFunc("/kms/listKeys", s.cors(s.chaosMiddleware(s.requireAuth(s.handleListKeys))))
	mux.HandleFunc("/kms/listUsers", s.cors(s.chaosMiddleware(s.requireAuth(s.handleListUsers))))
	mux.HandleFunc("/kms/auditLog", s.cors(s.chaosMiddleware(s.requireAuth(s.handleAuditLog))))
	mux.HandleFunc("/kms/encrypt", s.cors(s.chaosMiddleware(s.requireAuth(s.handleEncrypt))))
	mux.HandleFunc("/kms/decrypt", s.cors(s.chaosMiddleware(s.requireAuth(s.handleDecrypt))))

	// Status & cluster endpoints
	mux.HandleFunc("/status", s.cors(s.handleStatus))
	mux.HandleFunc("/cluster/status", s.cors(s.handleClusterStatus))
	
	// Dynamic Cluster Membership (Admin)
	mux.HandleFunc("/cluster/addNode", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleAddNode))))
	mux.HandleFunc("/cluster/removeNode", s.cors(s.chaosMiddleware(s.requireAdmin(s.handleRemoveNode))))

	// Events (SSE)
	mux.HandleFunc("/events", s.cors(s.handleEvents))
	mux.HandleFunc("/events/history", s.cors(s.handleEventsHistory))

	// Chaos endpoints (not affected by chaos middleware, but need CORS)
	mux.HandleFunc("/chaos/kill", s.cors(s.handleChaosKill))
	mux.HandleFunc("/chaos/revive", s.cors(s.handleChaosRevive))
	mux.HandleFunc("/chaos/delay", s.cors(s.handleChaosDelay))
	mux.HandleFunc("/chaos/drop", s.cors(s.handleChaosDropRate))
	mux.HandleFunc("/chaos/partition", s.cors(s.handleChaosPartition))
	mux.HandleFunc("/chaos/heal", s.cors(s.handleChaosHeal))

	log.Printf("[API] HTTP server starting on %s", s.address)
	return http.ListenAndServe(s.address, mux)
}

// cors adds CORS headers to all responses
func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

// chaosMiddleware wraps handlers with chaos module checks
func (s *Server) chaosMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.chaos.IsKilled() {
			http.Error(w, `{"error":"node is killed"}`, http.StatusServiceUnavailable)
			return
		}
		if s.chaos.ShouldDrop() {
			log.Printf("[CHAOS] Dropped request: %s %s", r.Method, r.URL.Path)
			http.Error(w, `{"error":"request dropped by chaos"}`, http.StatusServiceUnavailable)
			return
		}
		s.chaos.ApplyDelay()
		next(w, r)
	}
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// readJSON reads a JSON request body into the given struct
func readJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}
	defer r.Body.Close()
	return json.Unmarshal(body, v)
}

// --- SSE Event Streaming ---

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	subID, eventCh := s.events.Subscribe()
	defer s.events.Unsubscribe(subID)

	// Send initial connection event
	fmt.Fprintf(w, "data: {\"type\":\"CONNECTED\",\"node_id\":\"%s\"}\n\n", s.raftNode.GetID())
	flusher.Flush()

	for {
		select {
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleEventsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	events := s.events.GetAll()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
		"count":  len(events),
	})
}

// --- Raft Log Handler ---

func (s *Server) handleRaftLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logEntries := s.raftNode.FormatLogForDisplay()
	state := s.raftNode.GetState()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries":      logEntries,
		"count":        len(logEntries),
		"commit_index": state["commit_index"],
		"last_applied": state["last_applied"],
	})
}

// --- Cluster Status (aggregated) ---

func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type nodeStatus struct {
		NodeID      string `json:"node_id"`
		Address     string `json:"address"`
		Role        string `json:"role"`
		CurrentTerm int    `json:"current_term"`
		LeaderID    string `json:"leader_id"`
		CommitIndex int    `json:"commit_index"`
		LastApplied int    `json:"last_applied"`
		LogLength   int    `json:"log_length"`
		IsAlive     bool   `json:"is_alive"`
		IsChaosKilled bool `json:"is_chaos_killed"`
	}

	// Self status
	selfState := s.raftNode.GetState()
	nodes := []nodeStatus{
		{
			NodeID:      s.raftNode.GetID(),
			Address:     s.address,
			Role:        selfState["role"].(string),
			CurrentTerm: selfState["current_term"].(int),
			LeaderID:    selfState["leader_id"].(string),
			CommitIndex: selfState["commit_index"].(int),
			LastApplied: selfState["last_applied"].(int),
			LogLength:   selfState["log_length"].(int),
			IsAlive:     !s.chaos.IsKilled(),
			IsChaosKilled: s.chaos.IsKilled(),
		},
	}

	// Fetch peer statuses
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for _, peer := range s.raftNode.GetPeers() {
		ns := nodeStatus{
			Address: peer,
			IsAlive: false,
		}

		resp, err := client.Get(fmt.Sprintf("http://%s/status", peer))
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var peerState map[string]interface{}
			if json.Unmarshal(body, &peerState) == nil {
				ns.IsAlive = true
				if v, ok := peerState["node_id"].(string); ok {
					ns.NodeID = v
				}
				if v, ok := peerState["role"].(string); ok {
					ns.Role = v
				}
				if v, ok := peerState["current_term"].(float64); ok {
					ns.CurrentTerm = int(v)
				}
				if v, ok := peerState["leader_id"].(string); ok {
					ns.LeaderID = v
				}
				if v, ok := peerState["commit_index"].(float64); ok {
					ns.CommitIndex = int(v)
				}
				if v, ok := peerState["last_applied"].(float64); ok {
					ns.LastApplied = int(v)
				}
				if v, ok := peerState["log_length"].(float64); ok {
					ns.LogLength = int(v)
				}
				if chaosMap, ok := peerState["chaos"].(map[string]interface{}); ok {
					if killed, ok := chaosMap["killed"].(bool); ok {
						ns.IsChaosKilled = killed
					}
				}
			}
		}

		nodes = append(nodes, ns)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes":     nodes,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Raft RPC Handlers ---

func (s *Server) handleRequestVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var args raft.RequestVoteArgs
	if err := readJSON(r, &args); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if s.chaos.IsPartitioned(args.CandidateID) {
		http.Error(w, `{"error":"partitioned"}`, http.StatusServiceUnavailable)
		return
	}

	reply := s.raftNode.HandleRequestVote(args)
	writeJSON(w, http.StatusOK, reply)
}

func (s *Server) handleAppendEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var args raft.AppendEntriesArgs
	if err := readJSON(r, &args); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if s.chaos.IsPartitioned(args.LeaderID) {
		http.Error(w, `{"error":"partitioned"}`, http.StatusServiceUnavailable)
		return
	}

	reply := s.raftNode.HandleAppendEntries(args)
	writeJSON(w, http.StatusOK, reply)
}

// --- KMS Handlers ---

// redirectToLeader sends a 307 redirect to the leader node
func (s *Server) redirectToLeader(w http.ResponseWriter, r *http.Request) bool {
	if s.raftNode.IsLeader() {
		return false
	}

	leaderAddr := s.raftNode.GetLeaderAddress()
	if leaderAddr == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "no leader available",
		})
		return true
	}

	// For API calls from the dashboard, return JSON with leader info instead of redirect
	writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
		"error":        "not leader",
		"leader":       leaderAddr,
		"redirect_url": fmt.Sprintf("http://%s%s", leaderAddr, r.URL.Path),
	})
	return true
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		KeyID string `json:"key_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.KeyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key_id is required"})
		return
	}

	// Generate key material
	keyMaterial, err := kms.GenerateKeyMaterial()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	payload, _ := json.Marshal(kms.CreateKeyPayload{
		KeyID:       req.KeyID,
		KeyMaterial: keyMaterial,
		CreatedAt:   kms.Now(),
	})

	result, err := s.raftNode.SubmitCommand(storage.Command{
		Action:  "CREATE_KEY",
		Payload: payload,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.events.Add(raft.EventKeyCreated, s.raftNode.GetID(), 0, map[string]interface{}{"key_id": req.KeyID})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "key created",
		"key":     result,
	})
}

func (s *Server) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		KeyID string `json:"key_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	payload, _ := json.Marshal(kms.DeleteKeyPayload{KeyID: req.KeyID})

	result, err := s.raftNode.SubmitCommand(storage.Command{
		Action:  "DELETE_KEY",
		Payload: payload,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.events.Add(raft.EventKeyDeleted, s.raftNode.GetID(), 0, map[string]interface{}{"key_id": req.KeyID})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "key deleted",
		"key":     result,
	})
}

func (s *Server) handleRotateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		KeyID string `json:"key_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	keyMaterial, err := kms.GenerateKeyMaterial()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	payload, _ := json.Marshal(kms.RotateKeyPayload{
		KeyID:       req.KeyID,
		KeyMaterial: keyMaterial,
		CreatedAt:   kms.Now(),
	})

	result, err := s.raftNode.SubmitCommand(storage.Command{
		Action:  "ROTATE_KEY",
		Payload: payload,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.events.Add(raft.EventKeyRotated, s.raftNode.GetID(), 0, map[string]interface{}{"key_id": req.KeyID})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "key rotated",
		"key":     result,
	})
}

func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyID := r.URL.Query().Get("id")
	if keyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id query parameter is required"})
		return
	}

	key, err := s.kmsStore.GetKey(keyID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, key)
}

func (s *Server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keys := s.kmsStore.GetAllKeys()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"keys":  keys,
		"count": len(keys),
	})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	users := s.kmsStore.GetAllUsers()
	writeJSON(w, http.StatusOK, map[string]interface{}{"users": users})
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	trail := s.kmsStore.GetAuditTrail()
	writeJSON(w, http.StatusOK, map[string]interface{}{"audit_trail": trail})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		Username string   `json:"username"`
		Role     kms.Role `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.Username == "" || req.Role == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and role required"})
		return
	}

	apiKey, err := kms.GenerateKeyMaterial()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	payload := kms.CreateUserPayload{
		Username: req.Username,
		Role:     req.Role,
		APIKey:   apiKey,
	}

	data, _ := json.Marshal(payload)
	_, err = s.raftNode.SubmitCommand(storage.Command{
		Action:  "CREATE_USER",
		Payload: data,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	user, _ := s.kmsStore.GetUserByAPIKey(apiKey)
	s.events.Add(raft.EventKMSCommand, s.raftNode.GetID(), 0, map[string]interface{}{"action": "CREATE_USER", "username": req.Username})
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	payload := kms.DeleteUserPayload{Username: req.Username}
	data, _ := json.Marshal(payload)
	_, err := s.raftNode.SubmitCommand(storage.Command{
		Action:  "DELETE_USER",
		Payload: data,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.events.Add(raft.EventKMSCommand, s.raftNode.GetID(), 0, map[string]interface{}{"action": "DELETE_USER", "username": req.Username})
	writeJSON(w, http.StatusOK, map[string]string{"message": "user deleted"})
}

func (s *Server) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		KeyID     string `json:"key_id"`
		Plaintext string `json:"plaintext"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	ciphertext, err := s.kmsStore.Encrypt(req.KeyID, req.Plaintext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	user := r.Context().Value(userContextKey).(*kms.User)

	s.events.Add(raft.EventKMSCommand, s.raftNode.GetID(), 0, map[string]interface{}{"action": "ENCRYPT", "key_id": req.KeyID, "user": user.Username})

	// Submit Cryptographic Audit Log through Raft concurrently
	go func() {
		auditPayload := kms.AuditLogPayload{
			Entry: kms.AuditEntry{
				Timestamp: kms.Now(),
				Username:  user.Username,
				Action:    "ENCRYPT",
				KeyID:     req.KeyID,
			},
		}
		data, _ := json.Marshal(auditPayload)
		s.raftNode.SubmitCommand(storage.Command{Action: "AUDIT_LOG", Payload: data})
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"ciphertext": ciphertext,
	})
}

func (s *Server) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		KeyID      string `json:"key_id"`
		Ciphertext string `json:"ciphertext"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	plaintext, err := s.kmsStore.Decrypt(req.KeyID, req.Ciphertext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	user := r.Context().Value(userContextKey).(*kms.User)

	s.events.Add(raft.EventKMSCommand, s.raftNode.GetID(), 0, map[string]interface{}{"action": "DECRYPT", "key_id": req.KeyID, "user": user.Username})

	// Submit Cryptographic Audit Log through Raft concurrently
	go func() {
		auditPayload := kms.AuditLogPayload{
			Entry: kms.AuditEntry{
				Timestamp: kms.Now(),
				Username:  user.Username,
				Action:    "DECRYPT",
				KeyID:     req.KeyID,
			},
		}
		data, _ := json.Marshal(auditPayload)
		s.raftNode.SubmitCommand(storage.Command{Action: "AUDIT_LOG", Payload: data})
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"plaintext": plaintext,
	})
}

// Dynamic Cluster Membership Handlers

func (s *Server) handleAddNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		Address string `json:"address"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cmd := storage.Command{
		Action:  "ADD_NODE",
		Payload: []byte(req.Address),
	}
	_, err := s.raftNode.SubmitCommand(cmd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "ADD_NODE command submitted to cluster"})
}

func (s *Server) handleRemoveNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		Address string `json:"address"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cmd := storage.Command{
		Action:  "REMOVE_NODE",
		Payload: []byte(req.Address),
	}
	_, err := s.raftNode.SubmitCommand(cmd)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "REMOVE_NODE command submitted to cluster"})
}

// --- Status Handler ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := s.raftNode.GetState()
	state["chaos"] = s.chaos.GetStatus()
	state["address"] = s.address

	peersStatus := make(map[string]string)
	for _, peer := range s.raftNode.GetPeers() {
		peersStatus[peer] = "connected"
	}
	state["peers_status"] = peersStatus

	writeJSON(w, http.StatusOK, state)
}

// --- Chaos Handlers ---

func (s *Server) handleChaosKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.chaos.Kill()
	s.events.Add(raft.EventChaosKill, s.raftNode.GetID(), 0, map[string]interface{}{"message": "Node killed"})
	writeJSON(w, http.StatusOK, map[string]string{"message": "node killed"})
}

func (s *Server) handleChaosRevive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.chaos.Revive()
	s.events.Add(raft.EventChaosRevive, s.raftNode.GetID(), 0, map[string]interface{}{"message": "Node revived"})
	writeJSON(w, http.StatusOK, map[string]string{"message": "node revived"})
}

func (s *Server) handleChaosDelay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DelayMs int `json:"delay_ms"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.chaos.SetDelay(req.DelayMs)
	s.events.Add(raft.EventChaosDelay, s.raftNode.GetID(), 0, map[string]interface{}{"delay_ms": req.DelayMs})
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("delay set to %dms", req.DelayMs)})
}

func (s *Server) handleChaosDropRate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Rate float64 `json:"rate"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.chaos.SetDropRate(req.Rate)
	s.events.Add(raft.EventChaosDrop, s.raftNode.GetID(), 0, map[string]interface{}{"rate": req.Rate})
	writeJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("drop rate set to %.1f%%", req.Rate*100)})
}

func (s *Server) handleChaosPartition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Target string `json:"target"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.chaos.Partition(req.Target)
	s.events.Add(raft.EventRoleChange, s.raftNode.GetID(), 0, map[string]interface{}{"message": "Partitioned from " + req.Target})
	writeJSON(w, http.StatusOK, map[string]string{"message": "partitioned from " + req.Target})
}

func (s *Server) handleChaosHeal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Target string `json:"target"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.chaos.Heal(req.Target)
	s.events.Add(raft.EventRoleChange, s.raftNode.GetID(), 0, map[string]interface{}{"message": "Healed partition with " + req.Target})
	writeJSON(w, http.StatusOK, map[string]string{"message": "healed partition with " + req.Target})
}
