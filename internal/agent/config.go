// Package agent implements apage-agent: the lightweight service that runs next
// to a customer Agent runtime (spec §6). It enforces the allowlist, validates
// paths, serves the local register API, and maintains the tunnel connection.
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config is the persisted agent configuration written by `init` (spec §6.1).
type Config struct {
	InstanceID     string `json:"instanceId"`
	AgentType      string `json:"agentType"`
	Workspace      string `json:"workspace"` // allowlist root (spec §6.3)
	GatewayURL     string `json:"gatewayUrl"`
	APIURL         string `json:"apiUrl"`
	InstanceAPIKey string `json:"instanceApiKey,omitempty"`
	LocalPort      int    `json:"localPort"`
	// LocalToken is a random bearer that callers of the loopback register API must
	// present, so other local processes cannot register files (spec §6.3).
	LocalToken string `json:"localToken,omitempty"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apage")
}

func configPath(instance string) string { return filepath.Join(configDir(), instance+".json") }
func refsPath(instance string) string   { return filepath.Join(configDir(), instance+"-refs.json") }

// Save persists the config.
func (c *Config) Save() error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(configPath(c.InstanceID), b, 0o600)
}

// LoadConfig reads a config by instance name.
func LoadConfig(instance string) (*Config, error) {
	b, err := os.ReadFile(configPath(instance))
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// RefRecord maps an opaque fileRef to a canonical local path + metadata.
// The path never leaves the agent (spec §6.4).
type RefRecord struct {
	Path        string     `json:"path"`
	DisplayName string     `json:"displayName"`
	Size        int64      `json:"size"`
	MimeType    string     `json:"mimeType"`
	ModifiedAt  time.Time  `json:"modifiedAt"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// RefStore is a persistent fileRef -> path map, recoverable after restart
// (spec §File Ref rules).
type RefStore struct {
	mu       sync.RWMutex
	instance string
	refs     map[string]RefRecord
}

// LoadRefStore loads (or initializes) the ref map for an instance.
func LoadRefStore(instance string) *RefStore {
	rs := &RefStore{instance: instance, refs: map[string]RefRecord{}}
	if b, err := os.ReadFile(refsPath(instance)); err == nil {
		_ = json.Unmarshal(b, &rs.refs)
	}
	return rs
}

// Put stores a mapping and persists it.
func (rs *RefStore) Put(fileRef string, rec RefRecord) {
	rs.mu.Lock()
	rs.refs[fileRef] = rec
	rs.persistLocked()
	rs.mu.Unlock()
}

// Get resolves a fileRef, honoring expiry (spec §File Ref: expired refs cleaned).
func (rs *RefStore) Get(fileRef string) (RefRecord, bool) {
	rs.mu.RLock()
	rec, ok := rs.refs[fileRef]
	rs.mu.RUnlock()
	if ok && rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now()) {
		rs.mu.Lock()
		delete(rs.refs, fileRef)
		rs.persistLocked()
		rs.mu.Unlock()
		return RefRecord{}, false
	}
	return rec, ok
}

func (rs *RefStore) persistLocked() {
	_ = os.MkdirAll(configDir(), 0o700)
	b, _ := json.MarshalIndent(rs.refs, "", "  ")
	_ = os.WriteFile(refsPath(rs.instance), b, 0o600)
}
