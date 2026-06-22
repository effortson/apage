// Package agent implements apage-cli: the lightweight tool that runs next to a
// customer Agent runtime. It exposes an MCP server (spec §6) that an AI agent
// calls to upload files to the platform's cloud storage and create/manage cloud
// preview links. It also enforces the workspace allowlist and validates paths so
// the agent can only share files the operator has approved (see pathcheck.go).
package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// errMultipleInstances is returned when --instance must disambiguate.
var errMultipleInstances = errors.New("multiple instances configured; pass --instance")

// Config is the persisted apage-cli configuration written by `init`.
type Config struct {
	InstanceID     string `json:"instanceId"`
	AgentType      string `json:"agentType"`
	Workspace      string `json:"workspace"`            // allowlist root for uploads (spec §6.3)
	APIURL         string `json:"apiUrl"`               // platform API base, e.g. http://localhost:8080
	InstanceAPIKey string `json:"instanceApiKey"`       // bearer for the data plane
	MCPAddr        string `json:"mcpAddr"`              // MCP HTTP listen addr, e.g. 127.0.0.1:7777
	LocalToken     string `json:"localToken,omitempty"` // bearer the agent must present to the MCP endpoint
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".apage")
}

func configPath(instance string) string { return filepath.Join(configDir(), instance+".json") }

// Save persists the config.
func (c *Config) Save() error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(configPath(c.InstanceID), b, 0o600)
}

// LoadConfig reads a config by instance name. When instance is empty and exactly
// one config exists, it loads that one.
func LoadConfig(instance string) (*Config, error) {
	if instance == "" {
		only, err := soleInstance()
		if err != nil {
			return nil, err
		}
		instance = only
	}
	b, err := os.ReadFile(configPath(instance))
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.MCPAddr == "" {
		c.MCPAddr = "127.0.0.1:7777"
	}
	return &c, nil
}

// soleInstance returns the single configured instance name, or an error if there
// are zero or many (so the caller must pass --instance).
func soleInstance() (string, error) {
	entries, err := os.ReadDir(configDir())
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if filepath.Ext(n) == ".json" {
			names = append(names, n[:len(n)-len(".json")])
		}
	}
	if len(names) == 1 {
		return names[0], nil
	}
	if len(names) == 0 {
		return "", os.ErrNotExist
	}
	return "", errMultipleInstances
}
