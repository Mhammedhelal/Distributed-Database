package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	Nodes     NodesConfig     `json:"nodes"`
	Auth      AuthConfig      `json:"auth"`
	RateLimit RateLimitConfig `json:"rate_limit"`
}

type ServerConfig struct {
	Port            int      `json:"port"`
	ReadTimeout     Duration `json:"read_timeout"`
	WriteTimeout    Duration `json:"write_timeout"`
	ShutdownTimeout Duration `json:"shutdown_timeout"`
}

type NodesConfig struct {
	Master  NodeAddr   `json:"master"`
	Workers []NodeAddr `json:"workers"`
}

type NodeAddr struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type AuthConfig struct {
	HMACSecret   string   `json:"hmac_secret"`
	TokenTTL     Duration `json:"token_ttl"`
	ClientAPIKey string   `json:"client_api_key"`
}

type RateLimitConfig struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	Burst             int     `json:"burst"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = v
	return nil
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	cfg := defaults()
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

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
		return nil, fmt.Errorf("auth.hmac_secret is required (set via GATEWAY_HMAC_SECRET)")
	}

	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            8000,
			ReadTimeout:     Duration{15 * time.Second},
			WriteTimeout:    Duration{15 * time.Second},
			ShutdownTimeout: Duration{10 * time.Second},
		},
		Auth: AuthConfig{
			TokenTTL: Duration{30 * time.Second},
		},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             200,
		},
	}
}
