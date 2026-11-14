package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

type Config struct {
	HugoPath  string `json:"hugo_path"`
	Port      int    `json:"port"`
	AdminUser string `json:"admin_user"`
	AdminPass string `json:"admin_pass"` // Bcrypt hash
	// Story 3.1: OpenAI-compatible LLM endpoint (empty = LLM disabled)
	LLMBaseURL string `json:"llm_base_url"`
	LLMAPIKey  string `json:"llm_api_key"`
	LLMModel   string `json:"llm_model"`
	// Story 4.1: git identity for commits (defaults in internal/git)
	GitUser  string `json:"git_user"`
	GitEmail string `json:"git_email"`
	// Story 4.2: deploy push targets (empty = defaults main / hugo)
	BranchPages   string `json:"branch_pages"`
	BranchRelease string `json:"branch_release"`
}

var (
	cfg *Config
	mu  sync.RWMutex
	// Config path: HUGOSTACK_CONFIG env var, defaults to config.json in CWD.
	path = func() string {
		if p := os.Getenv("HUGOSTACK_CONFIG"); p != "" {
			return p
		}
		return "config.json"
	}()
)

// Path is where config.json lives (HUGOSTACK_CONFIG or CWD).
func Path() string { return path }

func Load() (*Config, error) {
	mu.RLock()
	if cfg != nil {
		defer mu.RUnlock()
		return cfg, nil
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.applyDefaults()
	if err := c.applyEnv(); err != nil {
		return nil, err
	}
	cfg = &c
	return cfg, nil
}

// applyDefaults fills branch targets so the deploy panel always has a ref.
func (c *Config) applyDefaults() {
	if c.BranchPages == "" {
		c.BranchPages = "main"
	}
	if c.BranchRelease == "" {
		c.BranchRelease = "hugo"
	}
}

// applyEnv lets the container environment win over the config file. In the
// Docker setup the file is ephemeral (rebootstrapped from env), so env
// overrides are the durable source. ponytail: explicit per-var, not a
// generic mapping table; upgrade path: table-driven if vars multiply.
func (c *Config) applyEnv() error {
	if p := os.Getenv("HUGOSTACK_PORT"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("HUGOSTACK_PORT: %w", err)
		}
		c.Port = v
	}
	if v := os.Getenv("HUGOSTACK_HUGO_PATH"); v != "" {
		c.HugoPath = v
	}
	if v := os.Getenv("HUGOSTACK_GIT_USER"); v != "" {
		c.GitUser = v
	}
	if v := os.Getenv("HUGOSTACK_GIT_EMAIL"); v != "" {
		c.GitEmail = v
	}
	return nil
}

func Save(c *Config) error {
	mu.Lock()
	defer mu.Unlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
