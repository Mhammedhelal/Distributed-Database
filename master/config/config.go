package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Config struct {
	Server    ServerConfig    `json:"server"`
	MySQL     MySQLConfig     `json:"mysql"`
	Cluster   ClusterConfig   `json:"cluster"`
	Auth      AuthConfig      `json:"auth"`
	WAL       WALConfig       `json:"wal"`
}

type ServerConfig struct {
	Port int `json:"port"`
}

type MySQLConfig struct {
	DSN string `json:"dsn"` // e.g. "root:pass@tcp(mysql:3306)/"
}

type ClusterConfig struct {
	NodeID           string   `json:"node_id"`
	SelfAddress      string   `json:"self_address"`
	WorkerAddresses  []string `json:"worker_addresses"`
	HeartbeatInterval duration `json:"heartbeat_interval"`
	MissThreshold    int      `json:"miss_threshold"`
}

type AuthConfig struct {
	HMACSecret string   `json:"hmac_secret"`
	TokenTTL   duration `json:"token_ttl"`
}

type WALConfig struct {
	Path string `json:"path"`
}

type duration struct{ time.Duration }

func (d *duration) UnmarshalJSON(b []byte) error {
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
	// Env overrides
	if v := os.Getenv("MYSQL_DSN"); v != "" {
		cfg.MySQL.DSN = v
	}
	if v := os.Getenv("HMAC_SECRET"); v != "" {
		cfg.Auth.HMACSecret = v
	}
	if v := os.Getenv("NODE_ID"); v != "" {
		cfg.Cluster.NodeID = v
	}
	if v := os.Getenv("SELF_ADDRESS"); v != "" {
		cfg.Cluster.SelfAddress = v
	}
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{Port: 8080},
		Cluster: ClusterConfig{
			NodeID:           "1",
			HeartbeatInterval: duration{5 * time.Second},
			MissThreshold:    3,
		},
		Auth: AuthConfig{
			HMACSecret: "change-me",
			TokenTTL:   duration{30 * time.Second},
		},
		WAL: WALConfig{Path: "/data/master.wal"},
	}
}

// Exported accessors so main.go doesn't reach into nested structs directly.
func (c *Config) HeartbeatInterval() time.Duration { return c.Cluster.HeartbeatInterval.Duration }
func (c *Config) TokenTTL() time.Duration          { return c.Auth.TokenTTL.Duration }