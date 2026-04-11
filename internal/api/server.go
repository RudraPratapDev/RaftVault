package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"raft-kms/internal/chaos"
	"raft-kms/internal/kms"
	"raft-kms/internal/raft"
	"raft-kms/internal/storage"
	"strings"
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
	mux.HandleFunc("/kms/keyMaterial", s.cors(s.chaosMiddleware(s.requireAuth(s.handleKeyMaterial))))
	mux.HandleFunc("/kms/exportKey", s.cors(s.chaosMiddleware(s.requireAuth(s.handleExportKey))))
	mux.HandleFunc("/kms/verifyChain", s.cors(s.chaosMiddleware(s.requireAuth(s.handleVerifyChain))))
	mux.HandleFunc("/kms/envelopeInfo", s.cors(s.chaosMiddleware(s.requireAuth(s.handleEnvelopeInfo))))

	// Public KMS endpoints
	mux.HandleFunc("/kms/login", s.cors(s.chaosMiddleware(s.handleLogin)))

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

	// Test/Demo endpoint — no auth required, localhost only
	mux.HandleFunc("/test/demo", s.cors(s.handleTestDemoPage))
	mux.HandleFunc("/test/demo/api", s.cors(s.handleTestDemo))
	mux.HandleFunc("/test/demo/createKey", s.cors(s.handleTestDemoCreateKey))
	mux.HandleFunc("/test/demo/encrypt", s.cors(s.handleTestDemoEncrypt))
	mux.HandleFunc("/test/demo/status", s.cors(s.handleTestDemoStatus))

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
		NodeID        string `json:"node_id"`
		Address       string `json:"address"`
		Role          string `json:"role"`
		CurrentTerm   int    `json:"current_term"`
		LeaderID      string `json:"leader_id"`
		CommitIndex   int    `json:"commit_index"`
		LastApplied   int    `json:"last_applied"`
		LogLength     int    `json:"log_length"`
		IsAlive       bool   `json:"is_alive"`
		IsChaosKilled bool   `json:"is_chaos_killed"`
	}

	// Self status
	selfState := s.raftNode.GetState()
	nodes := []nodeStatus{
		{
			NodeID:        s.raftNode.GetID(),
			Address:       s.address,
			Role:          selfState["role"].(string),
			CurrentTerm:   selfState["current_term"].(int),
			LeaderID:      selfState["leader_id"].(string),
			CommitIndex:   selfState["commit_index"].(int),
			LastApplied:   selfState["last_applied"].(int),
			LogLength:     selfState["log_length"].(int),
			IsAlive:       !s.chaos.IsKilled(),
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

// handleKeyMaterial returns the latest key version's raw material (base64) for security analysis.
// This is intentionally restricted to authenticated users — the material is sent to the
// local BitSecure analysis server, never stored or logged by the frontend.
func (s *Server) handleKeyMaterial(w http.ResponseWriter, r *http.Request) {
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
	if key.Status == "deleted" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "key is deleted"})
		return
	}
	if len(key.Versions) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key has no versions"})
		return
	}

	latest := key.Versions[len(key.Versions)-1]
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"key_id":       key.KeyID,
		"version":      latest.Version,
		"key_material": latest.KeyMaterial, // already base64-encoded
	})
}

func (s *Server) handleExportKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	key, err := s.kmsStore.GetKey(req.KeyID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if key.Status == "deleted" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "key is deleted"})
		return
	}
	if len(key.Versions) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key has no versions"})
		return
	}

	latest := key.Versions[len(key.Versions)-1]
	keyBytes, err := base64.StdEncoding.DecodeString(latest.KeyMaterial)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decode material"})
		return
	}

	// Parse PEM
	block, _ := pem.Decode([]byte(req.PublicKey))
	if block == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse PEM formatted public key"})
		return
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Try parsing as PKCS1
		pub, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse RSA public key: " + err.Error()})
			return
		}
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not an RSA public key"})
		return
	}

	wrappedBytes, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, rsaPub, keyBytes, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encryption failed: " + err.Error()})
		return
	}

	user := r.Context().Value(userContextKey).(*kms.User)

	s.events.Add(raft.EventKMSCommand, s.raftNode.GetID(), 0, map[string]interface{}{"action": "EXPORT", "key_id": req.KeyID, "user": user.Username})

	go func() {
		auditPayload := kms.AuditLogPayload{
			Entry: kms.AuditEntry{
				Timestamp: kms.Now(),
				Username:  user.Username,
				Action:    "EXPORT",
				KeyID:     req.KeyID,
			},
		}
		data, _ := json.Marshal(auditPayload)
		s.raftNode.SubmitCommand(storage.Command{Action: "AUDIT_LOG", Payload: data})
	}()

	writeJSON(w, http.StatusOK, map[string]string{
		"wrapped_key": base64.StdEncoding.EncodeToString(wrappedBytes),
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

func (s *Server) handleVerifyChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result := s.kmsStore.VerifyAuditChain()
	writeJSON(w, http.StatusOK, result)
}

// handleEnvelopeInfo performs a live envelope encryption and returns all intermediate
// values so the dashboard can visualize the KEK/DEK layers for demo purposes.
func (s *Server) handleEnvelopeInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	info, err := s.kmsStore.EnvelopeEncryptWithInfo(req.KeyID, req.Plaintext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
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

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.redirectToLeader(w, r) {
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"` // We treat API key as password
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	user, err := s.kmsStore.GetUserByAPIKey(req.Password)
	if err != nil || user.Username != req.Username {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "login successful",
		"token":   user.APIKey,
		"user":    user,
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

// --- Test/Demo Handlers (localhost only, no auth required) ---

// isLocalhost checks if the request comes from localhost
func isLocalhost(r *http.Request) bool {
	host := r.RemoteAddr
	return strings.HasPrefix(host, "127.0.0.1:") ||
		strings.HasPrefix(host, "[::1]:") ||
		strings.HasPrefix(host, "localhost:")
}

// handleTestDemoPage serves the interactive demo HTML page
func (s *Server) handleTestDemoPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, testDemoHTML)
}

// handleTestDemo serves the demo page info and cluster state (no auth)
func (s *Server) handleTestDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := s.raftNode.GetState()
	keys := s.kmsStore.GetAllKeys()
	users := s.kmsStore.GetAllUsers()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"node_id":      state["node_id"],
		"role":         state["role"],
		"current_term": state["current_term"],
		"leader_id":    state["leader_id"],
		"log_length":   state["log_length"],
		"commit_index": state["commit_index"],
		"keys":         keys,
		"users":        users,
		"admin_key":    "admin-secret-key",
		"hint":         "POST /test/demo/createKey with {\"key_id\":\"my-key\"} to create a key without auth",
	})
}

// handleTestDemoCreateKey creates a key without requiring auth (localhost only)
func (s *Server) handleTestDemoCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.raftNode.IsLeader() {
		leaderAddr := s.raftNode.GetLeaderAddress()
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error":  "not leader — send to leader",
			"leader": leaderAddr,
		})
		return
	}

	var req struct {
		KeyID string `json:"key_id"`
	}
	if err := readJSON(r, &req); err != nil || req.KeyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key_id is required"})
		return
	}

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

	s.events.Add(raft.EventKeyCreated, s.raftNode.GetID(), 0, map[string]interface{}{
		"key_id": req.KeyID, "source": "test/demo",
	})
	log.Printf("[TEST/DEMO] Key created via demo endpoint: %s", req.KeyID)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "key created via demo endpoint",
		"key":     result,
	})
}

// handleTestDemoEncrypt encrypts plaintext without auth (localhost only)
func (s *Server) handleTestDemoEncrypt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ciphertext": ciphertext,
		"key_id":     req.KeyID,
		"note":       "demo encryption — not audited through Raft",
	})
}

// handleTestDemoStatus returns full cluster status without auth (localhost only)
func (s *Server) handleTestDemoStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := s.raftNode.GetState()
	state["chaos"] = s.chaos.GetStatus()
	state["address"] = s.address
	state["events_count"] = len(s.events.GetAll())

	writeJSON(w, http.StatusOK, state)
}

// testDemoHTML is the embedded HTML for the /test/demo page
const testDemoHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>RaftKMS — Test Demo</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: 'Courier New', monospace; background: #0a0e1a; color: #e0e6f0; min-height: 100vh; padding: 24px; }
    h1 { color: #00d4ff; font-size: 22px; margin-bottom: 4px; }
    .subtitle { color: #6b7a99; font-size: 12px; margin-bottom: 28px; }
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; max-width: 1100px; }
    @media (max-width: 700px) { .grid { grid-template-columns: 1fr; } }
    .card { background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08); border-radius: 10px; padding: 18px; }
    .card h2 { font-size: 13px; color: #00d4ff; margin-bottom: 14px; text-transform: uppercase; letter-spacing: 1px; }
    label { display: block; font-size: 11px; color: #6b7a99; margin-bottom: 4px; margin-top: 10px; }
    input, select, textarea { width: 100%; background: rgba(0,0,0,0.4); border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; color: #e0e6f0; padding: 7px 10px; font-size: 12px; font-family: inherit; outline: none; }
    input:focus, select:focus, textarea:focus { border-color: #00d4ff; }
    textarea { min-height: 60px; resize: vertical; }
    button { margin-top: 12px; padding: 8px 16px; border: none; border-radius: 6px; font-size: 12px; font-family: inherit; cursor: pointer; font-weight: 600; transition: opacity 0.15s; }
    button:hover { opacity: 0.85; }
    .btn-blue { background: #0066cc; color: #fff; }
    .btn-green { background: #00aa55; color: #fff; }
    .btn-red { background: #cc2244; color: #fff; }
    .btn-orange { background: #cc6600; color: #fff; }
    .result { margin-top: 10px; padding: 10px; background: rgba(0,0,0,0.5); border-radius: 6px; font-size: 11px; word-break: break-all; white-space: pre-wrap; max-height: 160px; overflow-y: auto; border: 1px solid rgba(255,255,255,0.06); }
    .result.ok { border-color: rgba(0,200,100,0.3); color: #00cc66; }
    .result.err { border-color: rgba(200,50,50,0.3); color: #ff4466; }
    .node-row { display: flex; align-items: center; gap: 10px; padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,0.05); font-size: 12px; }
    .node-row:last-child { border-bottom: none; }
    .badge { padding: 2px 8px; border-radius: 4px; font-size: 10px; font-weight: 700; text-transform: uppercase; }
    .badge.leader { background: #00aa55; color: #fff; }
    .badge.follower { background: #0055aa; color: #fff; }
    .badge.candidate { background: #aa7700; color: #fff; }
    .badge.offline { background: #550022; color: #ff6688; }
    .log-area { background: rgba(0,0,0,0.6); border: 1px solid rgba(255,255,255,0.06); border-radius: 6px; padding: 10px; font-size: 11px; max-height: 200px; overflow-y: auto; color: #8899bb; }
    .log-line { padding: 2px 0; border-bottom: 1px solid rgba(255,255,255,0.03); }
    .log-line.election { color: #ffcc00; }
    .log-line.leader { color: #00cc66; }
    .log-line.vote { color: #00aaff; }
    .log-line.chaos { color: #ff4466; }
    .log-line.kms { color: #cc88ff; }
    .log-line.replication { color: #44aaff; }
    .log-line.commit { color: #00ffaa; }
    .stat { color: #6b7a99; font-size: 10px; }
    .stat span { color: #e0e6f0; }
    .key-item { padding: 5px 0; border-bottom: 1px solid rgba(255,255,255,0.04); font-size: 11px; display: flex; justify-content: space-between; }
    .key-item:last-child { border-bottom: none; }
    .empty { color: #444; font-size: 11px; padding: 8px 0; }
    .chaos-row { display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap; }
    .node-select { background: rgba(0,0,0,0.4); border: 1px solid rgba(255,255,255,0.1); border-radius: 6px; color: #e0e6f0; padding: 5px 8px; font-size: 11px; font-family: inherit; }
    #refreshBtn { background: rgba(255,255,255,0.07); color: #aab; border: 1px solid rgba(255,255,255,0.1); }
    .section-title { font-size: 10px; color: #6b7a99; text-transform: uppercase; letter-spacing: 1px; margin-bottom: 8px; }
  </style>
</head>
<body>
  <h1>⚡ RaftKMS — Test Demo</h1>
  <p class="subtitle">Testing panel · No authentication required · For demo and development use</p>
  <div class="grid">
    <div class="card">
      <h2>🖧 Cluster Status</h2>
      <div id="nodesContainer"><div class="empty">Loading...</div></div>
      <button id="refreshBtn" onclick="refreshAll()" style="margin-top:12px">↻ Refresh</button>
    </div>
    <div class="card">
      <h2>🔑 Create Key (No Auth)</h2>
      <label>Key ID</label>
      <input id="keyId" placeholder="e.g. user-alice-key" />
      <button class="btn-green" onclick="createKey()">Create Key via Raft</button>
      <div id="createKeyResult" class="result" style="display:none"></div>
      <div style="margin-top:20px">
        <div class="section-title">Active Keys</div>
        <div id="keysList"><div class="empty">No keys yet.</div></div>
      </div>
    </div>
    <div class="card">
      <h2>🔒 Encrypt / Decrypt</h2>
      <label>Key ID</label>
      <input id="encKeyId" placeholder="Key ID to use" />
      <label>Plaintext</label>
      <textarea id="encPlaintext" placeholder="Text to encrypt..."></textarea>
      <button class="btn-blue" onclick="doEncrypt()">Encrypt</button>
      <div id="encResult" class="result" style="display:none"></div>
      <div style="margin-top:16px">
        <label>Ciphertext to Decrypt</label>
        <textarea id="decCiphertext" placeholder="Paste ciphertext here..."></textarea>
        <label>Key ID</label>
        <input id="decKeyId" placeholder="Key ID used for encryption" />
        <button class="btn-orange" onclick="doDecrypt()">Decrypt</button>
        <div id="decResult" class="result" style="display:none"></div>
      </div>
    </div>
    <div class="card">
      <h2>💥 Chaos / Failover Simulation</h2>
      <label>Target Node</label>
      <select id="chaosNode" class="node-select" style="width:100%">

      </select>
      <div class="chaos-row">
        <button class="btn-red" onclick="chaosKill()">💀 Kill Node</button>
        <button class="btn-green" onclick="chaosRevive()">✅ Revive Node</button>
      </div>
      <div id="chaosResult" class="result" style="display:none"></div>
      <div style="margin-top:16px">
        <label>Delay (ms) — simulates slow network</label>
        <input id="delayMs" type="number" placeholder="e.g. 300" />
        <button class="btn-orange" onclick="setDelay()">Set Delay</button>
      </div>
    </div>
    <div class="card" style="grid-column: span 2">
      <h2>📋 Live Event Stream</h2>
      <div class="log-area" id="eventLog"><div class="log-line">Connecting to event streams...</div></div>
    </div>
  </div>
  <script>
    const HOST = window.location.hostname || 'localhost'
    const PORT = window.location.port || '5001'
    const NODES = [HOST + ':' + PORT]
    let leaderAddr = NODES[0]
    let eventSources = []
    function getCategory(type) {
      if (['ELECTION_START','LEADER_ELECTED'].includes(type)) return 'election'
      if (['VOTE_GRANTED','VOTE_REJECTED','REQUEST_VOTE'].includes(type)) return 'vote'
      if (['LOG_REPLICATED','APPEND_ENTRIES','HEARTBEAT'].includes(type)) return 'replication'
      if (['ENTRY_COMMITTED','ENTRY_APPLIED'].includes(type)) return 'commit'
      if (['CHAOS_KILL','CHAOS_REVIVE','CHAOS_DELAY','CHAOS_DROP','ROLE_CHANGE'].includes(type)) return 'chaos'
      if (type.includes('KMS')||['KEY_CREATED','KEY_DELETED','KEY_ROTATED','ENCRYPT','DECRYPT'].includes(type)) return 'kms'
      return 'replication'
    }
    function ts() { return new Date().toLocaleTimeString('en-US',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit',fractionalSecondDigits:1}) }
    function appendLog(text, category) {
      const el = document.getElementById('eventLog')
      const line = document.createElement('div')
      line.className = 'log-line ' + (category||'')
      line.textContent = '[' + ts() + '] ' + text
      el.appendChild(line)
      while (el.children.length > 200) el.removeChild(el.firstChild)
      el.scrollTop = el.scrollHeight
    }
    function connectSSE() {
      eventSources.forEach(es => es.close()); eventSources = []
      NODES.forEach(addr => {
        try {
          const es = new EventSource('http://' + addr + '/events')
          es.onmessage = (e) => {
            try {
              const ev = JSON.parse(e.data)
              if (ev.type === 'CONNECTED') { appendLog('Connected to ' + addr, 'replication'); return }
              const details = ev.details ? JSON.stringify(ev.details) : ''
              appendLog('[' + ev.node_id + '] ' + ev.type + ' T' + ev.term + ' ' + details, getCategory(ev.type))
            } catch {}
          }
          es.onerror = () => appendLog('SSE error on ' + addr, 'chaos')
          eventSources.push(es)
        } catch {}
      })
    }
    async function apiFetch(addr, path, method, body) {
      const opts = { method: method||'GET', headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer admin-secret-key' } }
      if (body) opts.body = JSON.stringify(body)
      const res = await fetch('http://' + addr + path, opts)
      const json = await res.json()
      if (!res.ok) throw new Error(json.error || JSON.stringify(json))
      return json
    }
    async function refreshAll() {
      let found = false
      for (const addr of NODES) {
        try {
          const data = await apiFetch(addr, '/cluster/status')
          if (data.nodes) {
            renderNodes(data.nodes)
            // Update NODES list from cluster status
            const allAddrs = data.nodes.map(n => n.address)
            NODES.length = 0; allAddrs.forEach(a => { if (!NODES.includes(a)) NODES.push(a) })
            // Update chaos dropdown
            const sel = document.getElementById('chaosNode'); sel.innerHTML = ''
            data.nodes.forEach(n => { const o = document.createElement('option'); o.value = n.address; o.textContent = n.node_id + ' (' + n.address + ')'; sel.appendChild(o) })
            const leader = data.nodes.find(n => n.role === 'LEADER' && n.is_alive && !n.is_chaos_killed)
            if (leader) leaderAddr = leader.address
            found = true; break
          }
        } catch {}
      }
      if (!found) document.getElementById('nodesContainer').innerHTML = '<div class="empty" style="color:#ff4466">Cannot reach any node. Is the cluster running?</div>'
      for (const addr of NODES) {
        try { const data = await apiFetch(addr, '/kms/listKeys'); renderKeys(data.keys||[]); break } catch {}
      }
    }
    function renderNodes(nodes) {
      const c = document.getElementById('nodesContainer'); c.innerHTML = ''
      nodes.forEach(n => {
        const alive = n.is_alive && !n.is_chaos_killed
        const role = alive ? n.role.toLowerCase() : 'offline'
        const row = document.createElement('div'); row.className = 'node-row'
        row.innerHTML = '<span class="badge ' + role + '">' + role.toUpperCase() + '</span><span style="flex:1;font-weight:600">' + n.node_id + '</span><span class="stat">T<span>' + n.current_term + '</span></span><span class="stat">Log:<span>' + n.log_length + '</span></span><span class="stat">CI:<span>' + n.commit_index + '</span></span>'
        c.appendChild(row)
      })
    }
    function renderKeys(keys) {
      const el = document.getElementById('keysList')
      if (!keys.length) { el.innerHTML = '<div class="empty">No keys yet.</div>'; return }
      el.innerHTML = keys.map(k => '<div class="key-item"><span style="color:#cc88ff">' + k.key_id + '</span><span class="stat">v<span>' + (k.versions&&k.versions.length||1) + '</span></span></div>').join('')
    }
    function showResult(id, data, ok) {
      const el = document.getElementById(id); el.style.display = 'block'
      el.className = 'result ' + (ok?'ok':'err')
      el.textContent = typeof data === 'string' ? data : JSON.stringify(data, null, 2)
    }
    async function createKey() {
      const keyId = document.getElementById('keyId').value.trim()
      if (!keyId) { showResult('createKeyResult','key_id is required',false); return }
      try {
        const data = await apiFetch(leaderAddr, '/test/demo/createKey', 'POST', { key_id: keyId })
        showResult('createKeyResult', data, true)
        document.getElementById('keyId').value = ''
        appendLog('Key created: ' + keyId, 'kms')
        setTimeout(refreshAll, 500)
      } catch (e) { showResult('createKeyResult', e.message, false) }
    }
    async function doEncrypt() {
      const keyId = document.getElementById('encKeyId').value.trim()
      const plaintext = document.getElementById('encPlaintext').value
      if (!keyId||!plaintext) { showResult('encResult','key_id and plaintext required',false); return }
      try {
        const data = await apiFetch(leaderAddr, '/test/demo/encrypt', 'POST', { key_id: keyId, plaintext })
        showResult('encResult', data.ciphertext, true)
        document.getElementById('decCiphertext').value = data.ciphertext
        document.getElementById('decKeyId').value = keyId
        appendLog('Encrypted with key: ' + keyId, 'kms')
      } catch (e) { showResult('encResult', e.message, false) }
    }
    async function doDecrypt() {
      const keyId = document.getElementById('decKeyId').value.trim()
      const ciphertext = document.getElementById('decCiphertext').value.trim()
      if (!keyId||!ciphertext) { showResult('decResult','key_id and ciphertext required',false); return }
      try {
        const data = await apiFetch(leaderAddr, '/kms/decrypt', 'POST', { key_id: keyId, ciphertext })
        showResult('decResult', data.plaintext, true)
        appendLog('Decrypted with key: ' + keyId, 'kms')
      } catch (e) { showResult('decResult', e.message, false) }
    }
    async function chaosKill() {
      const addr = document.getElementById('chaosNode').value
      try {
        const data = await fetch('http://' + addr + '/chaos/kill', {method:'POST'}).then(r=>r.json())
        showResult('chaosResult', data, true)
        appendLog('KILLED node at ' + addr, 'chaos')
        setTimeout(refreshAll, 1500)
      } catch (e) { showResult('chaosResult', e.message, false) }
    }
    async function chaosRevive() {
      const addr = document.getElementById('chaosNode').value
      try {
        const data = await fetch('http://' + addr + '/chaos/revive', {method:'POST'}).then(r=>r.json())
        showResult('chaosResult', data, true)
        appendLog('REVIVED node at ' + addr, 'chaos')
        setTimeout(refreshAll, 1500)
      } catch (e) { showResult('chaosResult', e.message, false) }
    }
    async function setDelay() {
      const addr = document.getElementById('chaosNode').value
      const ms = parseInt(document.getElementById('delayMs').value)||0
      try {
        const data = await fetch('http://' + addr + '/chaos/delay', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({delay_ms:ms})}).then(r=>r.json())
        showResult('chaosResult', data, true)
        appendLog('Set delay ' + ms + 'ms on ' + addr, 'chaos')
      } catch (e) { showResult('chaosResult', e.message, false) }
    }
    connectSSE(); refreshAll(); setInterval(refreshAll, 3000)
  </script>
</body>
</html>`
