package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all configuration for the CMDR service
type Config struct {
	// Service configuration
	APIPort  int    `envconfig:"API_PORT" default:"8080"`
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// OTLP Receiver configuration
	OTLPGRPCEndpoint string `envconfig:"OTLP_GRPC_ENDPOINT" default:"0.0.0.0:4317"`
	OTLPHTTPEndpoint string `envconfig:"OTLP_HTTP_ENDPOINT" default:"0.0.0.0:4318"`

	// Trace Fetcher configuration (optional - for historical traces)
	JaegerURL string `envconfig:"JAEGER_URL"`
	TempoURL  string `envconfig:"TEMPO_URL"`

	// Agentgateway Client configuration
	AgentgatewayURL     string        `envconfig:"AGENTGATEWAY_URL"`
	AgentgatewayTimeout time.Duration `envconfig:"AGENTGATEWAY_TIMEOUT" default:"60s"`
	AgentgatewayRetries int           `envconfig:"AGENTGATEWAY_RETRY_ATTEMPTS" default:"3"`

	// Freeze-Tools MCP configuration
	FreezeToolsPort int `envconfig:"FREEZETOOLS_PORT" default:"9090"`

	// Database configuration
	PostgresURL     string `envconfig:"POSTGRES_URL" required:"true"`
	PostgresMaxConn int    `envconfig:"POSTGRES_MAX_CONNS" default:"50"`

	// Replay Engine configuration
	WorkerPoolSize       int `envconfig:"WORKER_POOL_SIZE" default:"10"`
	MaxConcurrentReplays int `envconfig:"MAX_CONCURRENT_REPLAYS" default:"5"`

	// Evaluation configuration
	LLMJudgeModel string        `envconfig:"LLM_JUDGE_MODEL" default:"claude-3-5-sonnet-20241022"`
	EvalTimeout   time.Duration `envconfig:"EVAL_TIMEOUT" default:"120s"`

	// Authentication configuration (optional)
	JWTSecret string        `envconfig:"JWT_SECRET"`
	JWTExpiry time.Duration `envconfig:"JWT_EXPIRY" default:"24h"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("CMDR", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.APIPort < 1 || c.APIPort > 65535 {
		return fmt.Errorf("invalid API port: %d", c.APIPort)
	}

	if c.PostgresURL == "" {
		return fmt.Errorf("POSTGRES_URL is required")
	}

	if c.WorkerPoolSize < 1 {
		return fmt.Errorf("WORKER_POOL_SIZE must be at least 1")
	}

	if c.MaxConcurrentReplays < 1 {
		return fmt.Errorf("MAX_CONCURRENT_REPLAYS must be at least 1")
	}

	return nil
}

// RequireAgentgateway checks that agentgateway configuration is present.
// Call after Load() for commands that need agentgateway (serve, replay).
// Load() already calls Validate() for base config; this adds the agentgateway check.
func (c *Config) RequireAgentgateway() error {
	if c.AgentgatewayURL == "" {
		return fmt.Errorf("CMDR_AGENTGATEWAY_URL is required for this command")
	}
	return nil
}
