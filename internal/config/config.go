package config

import (
	"encoding/json"
	"os"
	"sync"
)

type Config struct {
	HugoPath  string `json:"hugo_path"`
	Port      int    `json:"port"`
	AdminUser string `json:"admin_user"`
	AdminPass string `json:"admin_pass"` // Bcrypt hash
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
	cfg = &c
	return cfg, nil
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
