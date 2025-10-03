package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Storage  StorageConfig  `yaml:"storage"`
	WAL      WALConfig      `yaml:"wal"`
	Queue    QueueConfig    `yaml:"queue"`
	Cluster  ClusterConfig  `yaml:"cluster"`
	Logging  LoggingConfig  `yaml:"logging"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	HTTPAddr string `yaml:"http_addr"`
	GRPCAddr string `yaml:"grpc_addr"`
}

// StorageConfig holds storage settings
type StorageConfig struct {
	DataDir string `yaml:"data_dir"`
}

// WALConfig holds WAL settings
type WALConfig struct {
	SegmentSize int64 `yaml:"segment_size"`
	Fsync       bool  `yaml:"fsync"`
}

// QueueConfig holds queue settings
type QueueConfig struct {
	Shards             int           `yaml:"shards"`
	LeaseCheckInterval time.Duration `yaml:"lease_check_interval"`
}

// ClusterConfig holds cluster settings
type ClusterConfig struct {
	Enabled     bool     `yaml:"enabled"`
	NodeID      string   `yaml:"node_id"`
	RaftAddr    string   `yaml:"raft_addr"`
	Bootstrap   bool     `yaml:"bootstrap"`
	SeedNodes   []string `yaml:"seed_nodes"`
	Replication int      `yaml:"replication"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // json or console
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			HTTPAddr: ":8080",
			GRPCAddr: ":9090",
		},
		Storage: StorageConfig{
			DataDir: "./data",
		},
		WAL: WALConfig{
			SegmentSize: 64 * 1024 * 1024, // 64MB
			Fsync:       true,
		},
		Queue: QueueConfig{
			Shards:             4,
			LeaseCheckInterval: 1 * time.Second,
		},
		Cluster: ClusterConfig{
			Enabled:     false,
			NodeID:      "",
			RaftAddr:    ":7000",
			Bootstrap:   false,
			SeedNodes:   []string{},
			Replication: 3,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "console",
		},
	}
}

// Load loads configuration from file
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if file doesn't exist
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// LoadOrDefault loads config from file or returns default
func LoadOrDefault(path string) *Config {
	if path == "" {
		return Default()
	}

	cfg, err := Load(path)
	if err != nil {
		fmt.Printf("Warning: failed to load config: %v, using defaults\n", err)
		return Default()
	}

	return cfg
}
