// Package config provides configuration loading and validation for kv-router.
// Configuration is loaded from YAML files and includes backend definitions,
// block sizing parameters, and scorer weights for routing decisions.
package config

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the kv-router process.
type Config struct {
	// ListenAddr is the address the router listens on (e.g. ":8080").
	ListenAddr string `yaml:"listen_addr"`

	// Backends defines the set of upstream LLM inference backends.
	Backends []BackendConfig `yaml:"backends"`

	// BlockSize is the number of characters per KV-cache block.
	// Determines the granularity of prefix matching and cache scoring.
	// Default: 64.
	BlockSize int `yaml:"block_size"`

	// Scorer holds the weights used by the routing scorer to rank backends.
	Scorer ScorerConfig `yaml:"scorer"`
}

// BackendConfig describes a single upstream inference backend.
type BackendConfig struct {
	// ID is a unique identifier for this backend (used in metrics and logs).
	ID string `yaml:"id"`

	// URL is the base URL of the backend's inference API.
	URL string `yaml:"url"`

	// CacheCapacityBlocks is the maximum number of blocks this backend's
	// KV-cache can hold. Used to estimate eviction risk.
	CacheCapacityBlocks int `yaml:"cache_capacity_blocks"`

	// HealthCheckInterval is how often the router probes this backend's health.
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`

	// MaxConcurrent is the maximum active requests accepted by this backend.
	MaxConcurrent int `yaml:"max_concurrent"`
}

// ScorerConfig holds the relative weights for the multi-factor routing scorer.
// Higher weight means that factor has more influence on backend selection.
type ScorerConfig struct {
	// QueueDepthWeight penalises backends with deeper request queues.
	QueueDepthWeight float64 `yaml:"queue_depth_weight"`

	// EvictionRiskWeight penalises backends whose caches are near capacity.
	EvictionRiskWeight float64 `yaml:"eviction_risk_weight"`

	// CacheHitWeight rewards backends that already hold relevant prefix blocks.
	CacheHitWeight float64 `yaml:"cache_hit_weight"`
}

const defaultBlockSize = 64

// LoadConfig reads a YAML configuration file from path and returns a validated
// Config. Missing block_size defaults to 64 characters.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %q: %w", path, err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	if cfg.BlockSize == 0 {
		cfg.BlockSize = defaultBlockSize
	}
	if cfg.Scorer.CacheHitWeight == 0 && cfg.Scorer.QueueDepthWeight == 0 && cfg.Scorer.EvictionRiskWeight == 0 {
		cfg.Scorer = ScorerConfig{CacheHitWeight: 1.0, QueueDepthWeight: 0.5, EvictionRiskWeight: 0.3}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate performs basic sanity checks on the loaded configuration.
func (c *Config) validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("config: listen_addr is required")
	}
	if len(c.Backends) == 0 {
		return fmt.Errorf("config: at least one backend is required")
	}
	if c.BlockSize <= 0 {
		return fmt.Errorf("config: block_size must be positive")
	}
	seenIDs := make(map[string]struct{}, len(c.Backends))
	for i, b := range c.Backends {
		if b.ID == "" {
			return fmt.Errorf("config: backend[%d]: id is required", i)
		}
		if b.URL == "" {
			return fmt.Errorf("config: backend[%d] (%s): url is required", i, b.ID)
		}
		u, err := url.Parse(b.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("config: backend[%d] (%s): url must be an absolute http(s) URL", i, b.ID)
		}
		if _, exists := seenIDs[b.ID]; exists {
			return fmt.Errorf("config: duplicate backend id %q", b.ID)
		}
		seenIDs[b.ID] = struct{}{}
		if b.CacheCapacityBlocks <= 0 {
			return fmt.Errorf("config: backend[%d] (%s): cache_capacity_blocks must be positive", i, b.ID)
		}
		if b.HealthCheckInterval < 0 {
			return fmt.Errorf("config: backend[%d] (%s): health_check_interval must not be negative", i, b.ID)
		}
		if b.MaxConcurrent < 0 {
			return fmt.Errorf("config: backend[%d] (%s): max_concurrent must not be negative", i, b.ID)
		}
	}
	for name, weight := range map[string]float64{"cache_hit_weight": c.Scorer.CacheHitWeight, "queue_depth_weight": c.Scorer.QueueDepthWeight, "eviction_risk_weight": c.Scorer.EvictionRiskWeight} {
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 {
			return fmt.Errorf("config: scorer.%s must be a finite non-negative number", name)
		}
	}
	return nil
}
