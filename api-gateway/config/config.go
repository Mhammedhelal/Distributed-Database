package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Nodes    NodesConfig    `yaml:"nodes"`
	Auth     AuthConfig     `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

type ServerConfig struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type NodesConfig struct {
	Master  NodeAddr   `yaml:"master"`
	Workers []NodeAddr `yaml:"workers"`
}

type NodeAddr struct {
	ID      string `yaml:"id"`
	Address string `yaml:"address"` // e.g. "http://master:8080"
}

type AuthConfig struct {
	// HMACSecret is the shared secret used to sign X-Master-Token.
	// Set via GATEWAY_HMAC_SECRET env var to avoid committing to VCS.
	HMACSecret string `yaml:"hmac_secret"`
	// TokenTTL is how long a generated token is considered valid.
	TokenTTL time.Duration `yaml:"token_ttl"`
	// ClientAPIKey is the optional bearer token external clients must present.
	// Leave empty to disable client auth.
	ClientAPIKey string `yaml:"client_api_key"`
}

type RateLimitConfig struct {
	// RequestsPerSecond is the steady-state rate per client IP.
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	// Burst is the maximum burst above the steady-state rate.
	Burst int `yaml:"burst"`
}

// Load reads config from path, then overrides with env vars.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	cfg := defaults()
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// Environment variable overrides (12-factor style).
	if v := os.Getenv("GATEWAY_HMAC_SECRET"); v != "" {
		cfg.Auth.HMACSecret = v
	}
	if v := os.Getenv("GATEWAY_CLIENT_API_KEY"); v != "" {
		cfg.Auth.ClientAPIKey = v
	}
	if v := os.Getenv("MASTER_ADDRESS"); v != "" {
		cfg.Nodes.Master.Address = v
	}

	if cfg.Auth.HMACSecret == "" {
		return nil, fmt.Errorf("auth.hmac_secret is required (set GATEWAY_HMAC_SECRET)")
	}

	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            8000,
			ReadTimeout:     15 * time.Second,
			WriteTimeout:    15 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Auth: AuthConfig{
			TokenTTL: 30 * time.Second,
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             200,
		},
	}
}