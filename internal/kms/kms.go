package kms

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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
	Timestamp    string `json:"timestamp"`
	Username     string `json:"username"`
	Action       string `json:"action"` // "ENCRYPT" or "DECRYPT"
	KeyID        string `json:"key_id"`
	PreviousHash string `json:"previous_hash"`
	CurrentHash  string `json:"current_hash"`
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
	mu           sync.RWMutex
	keys         map[string]*Key
	users        map[string]*User // Keyed by Username
	apiKeys      map[string]*User // Keyed by APIKey for fast auth
	auditTrail   []AuditEntry
	auditHMACKey []byte
	lastHash     string
}

// NewKMSStore creates a new KMSStore
func NewKMSStore() *KMSStore {
	store := &KMSStore{
		keys:         make(map[string]*Key),
		users:        make(map[string]*User),
		apiKeys:      make(map[string]*User),
		auditTrail:   make([]AuditEntry, 0),
		auditHMACKey: []byte("raft-kms-audit-hmac-secret-key-12345"),
		lastHash:     "0000000000000000000000000000000000000000000000000000000000000000",
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
	p.Entry.PreviousHash = s.lastHash
	mac := hmac.New(sha256.New, s.auditHMACKey)

	// Create canonical payload data for hashing (without CurrentHash)
	data := fmt.Sprintf("%s|%s|%s|%s|%s", p.Entry.PreviousHash, p.Entry.Timestamp, p.Entry.Username, p.Entry.Action, p.Entry.KeyID)
	mac.Write([]byte(data))

	p.Entry.CurrentHash = fmt.Sprintf("%x", mac.Sum(nil))
	s.lastHash = p.Entry.CurrentHash

	s.auditTrail = append(s.auditTrail, p.Entry)
	log.Printf("[KMS/AUDIT] User %s performed %s on Key %s (Hash: %s)", p.Entry.Username, p.Entry.Action, p.Entry.KeyID, p.Entry.CurrentHash[:8])
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

// ChainVerificationResult holds the result of verifying the audit chain
type ChainVerificationResult struct {
	Valid        bool   `json:"valid"`
	TotalEntries int    `json:"total_entries"`
	BrokenAt     int    `json:"broken_at"` // -1 if intact
	Message      string `json:"message"`
}

// VerifyAuditChain re-computes every HMAC in the chain and checks linkage
func (s *KMSStore) VerifyAuditChain() ChainVerificationResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.auditTrail) == 0 {
		return ChainVerificationResult{Valid: true, TotalEntries: 0, BrokenAt: -1, Message: "Chain is empty — nothing to verify."}
	}

	genesis := "0000000000000000000000000000000000000000000000000000000000000000"
	prevHash := genesis

	for i, entry := range s.auditTrail {
		// Verify the previous hash pointer is correct
		if entry.PreviousHash != prevHash {
			return ChainVerificationResult{
				Valid:        false,
				TotalEntries: len(s.auditTrail),
				BrokenAt:     i,
				Message:      fmt.Sprintf("Chain broken at entry %d: previous_hash mismatch", i),
			}
		}

		// Re-compute the HMAC for this entry
		mac := hmac.New(sha256.New, s.auditHMACKey)
		data := fmt.Sprintf("%s|%s|%s|%s|%s", entry.PreviousHash, entry.Timestamp, entry.Username, entry.Action, entry.KeyID)
		mac.Write([]byte(data))
		expected := fmt.Sprintf("%x", mac.Sum(nil))

		if entry.CurrentHash != expected {
			return ChainVerificationResult{
				Valid:        false,
				TotalEntries: len(s.auditTrail),
				BrokenAt:     i,
				Message:      fmt.Sprintf("Chain broken at entry %d: HMAC mismatch — entry may have been tampered with", i),
			}
		}

		prevHash = entry.CurrentHash
	}

	return ChainVerificationResult{
		Valid:        true,
		TotalEntries: len(s.auditTrail),
		BrokenAt:     -1,
		Message:      fmt.Sprintf("All %d entries verified. Chain is intact.", len(s.auditTrail)),
	}
}

// hkdfExtract implements HKDF-Extract
func hkdfExtract(salt, ikm []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write(ikm)
	return mac.Sum(nil)
}

// hkdfExpand implements HKDF-Expand
func hkdfExpand(prk, info []byte, length int) []byte {
	var okm []byte
	var t []byte
	var i byte = 1
	for len(okm) < length {
		mac := hmac.New(sha256.New, prk)
		mac.Write(t)
		mac.Write(info)
		mac.Write([]byte{i})
		t = mac.Sum(nil)
		okm = append(okm, t...)
		i++
	}
	return okm[:length]
}

// HKDF derives a key from master secret
func HKDF(masterSecret, salt, info []byte, length int) []byte {
	prk := hkdfExtract(salt, masterSecret)
	return hkdfExpand(prk, info, length)
}

// GenerateKeyMaterial generates a random 256-bit master secret and returns it base64-encoded
func GenerateKeyMaterial() (string, error) {
	key := make([]byte, 32) // 256 bits
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("failed to generate key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt encrypts plaintext using the latest version of the specified key via Envelope Encryption
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

	masterBytes, err := base64.StdEncoding.DecodeString(latestVersion.KeyMaterial)
	if err != nil {
		return "", fmt.Errorf("failed to decode key material: %w", err)
	}

	// 1. Derive KEK using HKDF
	info := []byte(fmt.Sprintf("%s:%d:%s", key.KeyID, latestVersion.Version, latestVersion.CreatedAt))
	kek := HKDF(masterBytes, nil, info, 32)
	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return "", fmt.Errorf("failed to create KEK cipher: %w", err)
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return "", fmt.Errorf("failed to create KEK GCM: %w", err)
	}

	// 2. Generate random DEK
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return "", fmt.Errorf("failed to generate DEK: %w", err)
	}

	// 3. Wrap DEK with KEK
	kekNonce := make([]byte, kekGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, kekNonce); err != nil {
		return "", fmt.Errorf("failed to generate KEK nonce: %w", err)
	}
	wrappedDEK := kekGCM.Seal(kekNonce, kekNonce, dek, nil) // Nonce is prepended to wrapped key

	// 4. Encrypt data with DEK
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("failed to create DEK cipher: %w", err)
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return "", fmt.Errorf("failed to create DEK GCM: %w", err)
	}
	dekNonce := make([]byte, dekGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, dekNonce); err != nil {
		return "", fmt.Errorf("failed to generate DEK nonce: %w", err)
	}
	ciphertext := dekGCM.Seal(dekNonce, dekNonce, []byte(plaintext), nil) // Nonce prepended

	// Format: version(4) + wrappedDEKLen(2) + wrappedDEK + ciphertext
	versionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBytes, uint32(latestVersion.Version))

	wrappedDEKLen := make([]byte, 2)
	binary.BigEndian.PutUint16(wrappedDEKLen, uint16(len(wrappedDEK)))

	combined := append(versionBytes, wrappedDEKLen...)
	combined = append(combined, wrappedDEK...)
	combined = append(combined, ciphertext...)

	return base64.StdEncoding.EncodeToString(combined), nil
}

// Decrypt decrypts ciphertext via Envelope Encryption, automatically detecting the key version used
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

	if len(combined) < 6 {
		return "", fmt.Errorf("ciphertext too short")
	}

	// Extract version and lengths
	version := int(binary.BigEndian.Uint32(combined[:4]))
	wrappedDEKLen := int(binary.BigEndian.Uint16(combined[4:6]))

	if 6+wrappedDEKLen > len(combined) {
		return "", fmt.Errorf("malformed ciphertext: wrapped DEK length exceeds bounds")
	}

	wrappedDEKFull := combined[6 : 6+wrappedDEKLen]
	ciphertextFull := combined[6+wrappedDEKLen:]

	// Find the key version
	if version < 1 || version > len(key.Versions) {
		return "", fmt.Errorf("key version %d not found", version)
	}

	keyVersion := key.Versions[version-1]
	masterBytes, err := base64.StdEncoding.DecodeString(keyVersion.KeyMaterial)
	if err != nil {
		return "", fmt.Errorf("failed to decode master material: %w", err)
	}

	// 1. Derive KEK via HKDF
	info := []byte(fmt.Sprintf("%s:%d:%s", key.KeyID, keyVersion.Version, keyVersion.CreatedAt))
	kek := HKDF(masterBytes, nil, info, 32)
	kekBlock, err := aes.NewCipher(kek)
	if err != nil {
		return "", fmt.Errorf("failed to create KEK cipher: %w", err)
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return "", fmt.Errorf("failed to create KEK GCM: %w", err)
	}

	// 2. Unwrap DEK
	kekNonceSize := kekGCM.NonceSize()
	if len(wrappedDEKFull) < kekNonceSize {
		return "", fmt.Errorf("wrapped DEK too short")
	}
	kekNonce, wrappedDEKBytes := wrappedDEKFull[:kekNonceSize], wrappedDEKFull[kekNonceSize:]
	dek, err := kekGCM.Open(nil, kekNonce, wrappedDEKBytes, nil)
	if err != nil {
		return "", fmt.Errorf("failed to unwrap DEK: %w", err)
	}

	// 3. Decrypt payload with DEK
	dekBlock, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("failed to create DEK cipher: %w", err)
	}
	dekGCM, err := cipher.NewGCM(dekBlock)
	if err != nil {
		return "", fmt.Errorf("failed to create DEK GCM: %w", err)
	}

	dekNonceSize := dekGCM.NonceSize()
	if len(ciphertextFull) < dekNonceSize {
		return "", fmt.Errorf("ciphertext payload too short")
	}

	dekNonce, ciphertextBytes := ciphertextFull[:dekNonceSize], ciphertextFull[dekNonceSize:]
	plaintext, err := dekGCM.Open(nil, dekNonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	return string(plaintext), nil
}

// Now returns current time as ISO string
func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// EnvelopeInfo holds all intermediate values from an envelope encryption for demo/visualization
type EnvelopeInfo struct {
	KeyID      string `json:"key_id"`
	KeyVersion int    `json:"key_version"`
	// HKDF
	HKDFInfo string `json:"hkdf_info"` // context string used as HKDF info param
	KEKHex   string `json:"kek_hex"`   // first 8 bytes of derived KEK (hex, truncated for display)
	// DEK
	DEKHex        string `json:"dek_hex"`         // first 8 bytes of random DEK (hex, truncated)
	WrappedDEKB64 string `json:"wrapped_dek_b64"` // AES-GCM(KEK, DEK) base64
	// Ciphertext
	CiphertextB64  string `json:"ciphertext_b64"`   // AES-GCM(DEK, plaintext) base64
	FinalOutputB64 string `json:"final_output_b64"` // full envelope output
	// Sizes
	PlaintextLen  int `json:"plaintext_len"`
	CiphertextLen int `json:"ciphertext_len"`
	WrappedDEKLen int `json:"wrapped_dek_len"`
}

// EnvelopeEncryptWithInfo encrypts and returns all intermediate values for visualization
func (s *KMSStore) EnvelopeEncryptWithInfo(keyID string, plaintext string) (*EnvelopeInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[keyID]
	if !exists {
		return nil, fmt.Errorf("key %s not found", keyID)
	}
	if key.Status == "deleted" {
		return nil, fmt.Errorf("key %s is deleted", keyID)
	}

	latestVersion := key.Versions[len(key.Versions)-1]
	masterBytes, err := base64.StdEncoding.DecodeString(latestVersion.KeyMaterial)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key material: %w", err)
	}

	// 1. Derive KEK using HKDF
	hkdfInfoStr := fmt.Sprintf("%s:%d:%s", key.KeyID, latestVersion.Version, latestVersion.CreatedAt)
	info := []byte(hkdfInfoStr)
	kek := HKDF(masterBytes, nil, info, 32)

	kekBlock, _ := aes.NewCipher(kek)
	kekGCM, _ := cipher.NewGCM(kekBlock)

	// 2. Generate random DEK
	dek := make([]byte, 32)
	io.ReadFull(rand.Reader, dek)

	// 3. Wrap DEK with KEK
	kekNonce := make([]byte, kekGCM.NonceSize())
	io.ReadFull(rand.Reader, kekNonce)
	wrappedDEK := kekGCM.Seal(kekNonce, kekNonce, dek, nil)

	// 4. Encrypt data with DEK
	dekBlock, _ := aes.NewCipher(dek)
	dekGCM, _ := cipher.NewGCM(dekBlock)
	dekNonce := make([]byte, dekGCM.NonceSize())
	io.ReadFull(rand.Reader, dekNonce)
	ciphertext := dekGCM.Seal(dekNonce, dekNonce, []byte(plaintext), nil)

	// 5. Assemble final output
	versionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(versionBytes, uint32(latestVersion.Version))
	wrappedDEKLen := make([]byte, 2)
	binary.BigEndian.PutUint16(wrappedDEKLen, uint16(len(wrappedDEK)))
	combined := append(versionBytes, wrappedDEKLen...)
	combined = append(combined, wrappedDEK...)
	combined = append(combined, ciphertext...)

	return &EnvelopeInfo{
		KeyID:          keyID,
		KeyVersion:     latestVersion.Version,
		HKDFInfo:       hkdfInfoStr,
		KEKHex:         fmt.Sprintf("%x...", kek[:8]),
		DEKHex:         fmt.Sprintf("%x...", dek[:8]),
		WrappedDEKB64:  base64.StdEncoding.EncodeToString(wrappedDEK),
		CiphertextB64:  base64.StdEncoding.EncodeToString(ciphertext),
		FinalOutputB64: base64.StdEncoding.EncodeToString(combined),
		PlaintextLen:   len(plaintext),
		CiphertextLen:  len(combined),
		WrappedDEKLen:  len(wrappedDEK),
	}, nil
}
