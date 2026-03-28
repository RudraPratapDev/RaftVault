package kms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"raft-kms/internal/storage"
	"sync"
	"time"
)

// Role and User definitions for RBAC
type Role string

const (
	RoleAdmin   Role = "admin"
	RoleService Role = "service"
)

type User struct {
	Username  string `json:"username"`
	Role      Role   `json:"role"`
	APIKey    string `json:"api_key"`
	CreatedAt string `json:"created_at"`
}

// AuditEntry represents an unalterable record of cryptographic operation
type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Username  string `json:"username"`
	Action    string `json:"action"` // "ENCRYPT" or "DECRYPT"
	KeyID     string `json:"key_id"`
}

// KeyVersion represents a specific version of a key
type KeyVersion struct {
	Version     int    `json:"version"`
	KeyMaterial string `json:"key_material"` // base64-encoded
	CreatedAt   string `json:"created_at"`
}

// Key represents a managed key with versions
type Key struct {
	KeyID     string       `json:"key_id"`
	CreatedAt string       `json:"created_at"`
	Versions  []KeyVersion `json:"versions"`
	Status    string       `json:"status"` // "active" | "deleted"
}

// CreateKeyPayload is the payload for a CREATE_KEY command
type CreateKeyPayload struct {
	KeyID       string `json:"key_id"`
	KeyMaterial string `json:"key_material"`
	CreatedAt   string `json:"created_at"`
}

// DeleteKeyPayload is the payload for a DELETE_KEY command
type DeleteKeyPayload struct {
	KeyID string `json:"key_id"`
}

// RotateKeyPayload is the payload for a ROTATE_KEY command
type RotateKeyPayload struct {
	KeyID       string `json:"key_id"`
	KeyMaterial string `json:"key_material"`
	CreatedAt   string `json:"created_at"`
}

// CreateUserPayload is the payload for CREATE_USER
type CreateUserPayload struct {
	Username string `json:"username"`
	Role     Role   `json:"role"`
	APIKey   string `json:"api_key"`
}

// DeleteUserPayload is the payload for DELETE_USER
type DeleteUserPayload struct {
	Username string `json:"username"`
}

// AuditLogPayload is the payload for AUDIT_LOG
type AuditLogPayload struct {
	Entry AuditEntry `json:"entry"`
}

// KMSStore is the in-memory key management store (state machine)
type KMSStore struct {
	mu         sync.RWMutex
	keys       map[string]*Key
	users      map[string]*User // Keyed by Username
	apiKeys    map[string]*User // Keyed by APIKey for fast auth
	auditTrail []AuditEntry
}

// NewKMSStore creates a new KMSStore
func NewKMSStore() *KMSStore {
	store := &KMSStore{
		keys:       make(map[string]*Key),
		users:      make(map[string]*User),
		apiKeys:    make(map[string]*User),
		auditTrail: make([]AuditEntry, 0),
	}

	// Bootstrap default admin identity
	admin := &User{
		Username:  "admin",
		Role:      RoleAdmin,
		APIKey:    "admin-secret-key",
		CreatedAt: Now(),
	}
	store.users[admin.Username] = admin
	store.apiKeys[admin.APIKey] = admin

	return store
}

// Apply applies a Raft command to the KMS state machine
func (s *KMSStore) Apply(command storage.Command) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch command.Action {
	case "CREATE_KEY":
		var payload CreateKeyPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, fmt.Errorf("invalid CREATE_KEY payload: %w", err)
		}
		return s.applyCreateKey(payload)

	case "DELETE_KEY":
		var payload DeleteKeyPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, fmt.Errorf("invalid DELETE_KEY payload: %w", err)
		}
		return s.applyDeleteKey(payload)

	case "ROTATE_KEY":
		var payload RotateKeyPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, fmt.Errorf("invalid ROTATE_KEY payload: %w", err)
		}
		return s.applyRotateKey(payload)

	case "CREATE_USER":
		var payload CreateUserPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, fmt.Errorf("invalid CREATE_USER payload: %w", err)
		}
		return s.applyCreateUser(payload)

	case "DELETE_USER":
		var payload DeleteUserPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, fmt.Errorf("invalid DELETE_USER payload: %w", err)
		}
		return s.applyDeleteUser(payload)

	case "AUDIT_LOG":
		var payload AuditLogPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, fmt.Errorf("invalid AUDIT_LOG payload: %w", err)
		}
		return s.applyAuditLog(payload)

	default:
		return nil, fmt.Errorf("unknown action: %s", command.Action)
	}
}

func (s *KMSStore) applyCreateKey(p CreateKeyPayload) (interface{}, error) {
	if _, exists := s.keys[p.KeyID]; exists {
		return nil, fmt.Errorf("key %s already exists", p.KeyID)
	}

	key := &Key{
		KeyID:     p.KeyID,
		CreatedAt: p.CreatedAt,
		Status:    "active",
		Versions: []KeyVersion{
			{
				Version:     1,
				KeyMaterial: p.KeyMaterial,
				CreatedAt:   p.CreatedAt,
			},
		},
	}
	s.keys[p.KeyID] = key
	log.Printf("[KMS] Created key: %s", p.KeyID)
	return key, nil
}

func (s *KMSStore) applyDeleteKey(p DeleteKeyPayload) (interface{}, error) {
	key, exists := s.keys[p.KeyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", p.KeyID)
	}
	key.Status = "deleted"
	log.Printf("[KMS] Deleted key: %s", p.KeyID)
	return key, nil
}

func (s *KMSStore) applyRotateKey(p RotateKeyPayload) (interface{}, error) {
	key, exists := s.keys[p.KeyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", p.KeyID)
	}
	if key.Status == "deleted" {
		return nil, fmt.Errorf("key %s is deleted", p.KeyID)
	}

	newVersion := KeyVersion{
		Version:     len(key.Versions) + 1,
		KeyMaterial: p.KeyMaterial,
		CreatedAt:   p.CreatedAt,
	}
	key.Versions = append(key.Versions, newVersion)
	log.Printf("[KMS] Rotated key: %s to version %d", p.KeyID, newVersion.Version)
	return key, nil
}

func (s *KMSStore) applyCreateUser(p CreateUserPayload) (interface{}, error) {
	if _, exists := s.users[p.Username]; exists {
		return nil, fmt.Errorf("user %s already exists", p.Username)
	}
	if _, exists := s.apiKeys[p.APIKey]; exists {
		return nil, fmt.Errorf("api key already in use")
	}

	user := &User{
		Username:  p.Username,
		Role:      p.Role,
		APIKey:    p.APIKey,
		CreatedAt: Now(),
	}
	s.users[p.Username] = user
	s.apiKeys[p.APIKey] = user
	log.Printf("[KMS] Created user: %s (Role: %s)", p.Username, p.Role)
	return user, nil
}

func (s *KMSStore) applyDeleteUser(p DeleteUserPayload) (interface{}, error) {
	user, exists := s.users[p.Username]
	if !exists {
		return nil, fmt.Errorf("user %s not found", p.Username)
	}
	if user.Username == "admin" {
		return nil, fmt.Errorf("cannot delete default admin user")
	}

	delete(s.apiKeys, user.APIKey)
	delete(s.users, p.Username)
	log.Printf("[KMS] Deleted user: %s", p.Username)
	return user, nil
}

func (s *KMSStore) applyAuditLog(p AuditLogPayload) (interface{}, error) {
	s.auditTrail = append(s.auditTrail, p.Entry)
	log.Printf("[KMS/AUDIT] User %s performed %s on Key %s", p.Entry.Username, p.Entry.Action, p.Entry.KeyID)
	return p.Entry, nil
}

// GetKey returns a key by ID (read-only, no Raft needed)
func (s *KMSStore) GetKey(keyID string) (*Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}
	return key, nil
}

// GetAllKeys returns all keys
func (s *KMSStore) GetAllKeys() []*Key {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]*Key, 0, len(s.keys))
	for _, k := range s.keys {
		keys = append(keys, k)
	}
	return keys
}

// GetUserByAPIKey authenticates an API key and returns the associated Identity
func (s *KMSStore) GetUserByAPIKey(apiKey string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.apiKeys[apiKey]
	if !exists {
		return nil, fmt.Errorf("invalid api key")
	}
	return user, nil
}

// GetAllUsers returns all users
func (s *KMSStore) GetAllUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	return users
}

// GetAuditTrail returns the immutable audit trail
func (s *KMSStore) GetAuditTrail() []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	trail := make([]AuditEntry, len(s.auditTrail))
	copy(trail, s.auditTrail)
	return trail
}

// GenerateKeyMaterial generates a random 256-bit AES key and returns it base64-encoded
func GenerateKeyMaterial() (string, error) {
	key := make([]byte, 32) // 256 bits
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt encrypts plaintext using the latest version of the specified key
func (s *KMSStore) Encrypt(keyID string, plaintext string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[keyID]
	if !exists {
		return "", fmt.Errorf("key %s not found", keyID)
	}
	if key.Status == "deleted" {
		return "", fmt.Errorf("key %s is deleted", keyID)
	}

	latestVersion := key.Versions[len(key.Versions)-1]

	keyBytes, err := base64.StdEncoding.DecodeString(latestVersion.KeyMaterial)
	if err != nil {
		return "", fmt.Errorf("failed to decode key material: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Prepend version number (4 bytes big endian) to ciphertext for version tracking
	versionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBytes, uint32(latestVersion.Version))

	ciphertext := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)

	// Format: version_bytes + ciphertext (with nonce prepended)
	combined := append(versionBytes, ciphertext...)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts ciphertext, automatically detecting the key version used
func (s *KMSStore) Decrypt(keyID string, ciphertextB64 string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[keyID]
	if !exists {
		return "", fmt.Errorf("key %s not found", keyID)
	}

	combined, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	if len(combined) < 4 {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract version
	version := int(binary.BigEndian.Uint32(combined[:4]))
	ciphertext := combined[4:]

	// Find the key version
	if version < 1 || version > len(key.Versions) {
		return "", fmt.Errorf("key version %d not found", version)
	}

	keyVersion := key.Versions[version-1]
	keyBytes, err := base64.StdEncoding.DecodeString(keyVersion.KeyMaterial)
	if err != nil {
		return "", fmt.Errorf("failed to decode key material: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short for nonce")
	}

	nonce, ciphertextBytes := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// Now returns current time as ISO string
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
