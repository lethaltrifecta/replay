package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name: "valid config with required fields",
			envVars: map[string]string{
				"CMDR_POSTGRES_URL":      "postgres://user:pass@localhost:5432/cmdr",
				"CMDR_AGENTGATEWAY_URL": "http://localhost:8080",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 8080, cfg.APIPort)
				assert.Equal(t, "info", cfg.LogLevel)
				assert.Equal(t, "postgres://user:pass@localhost:5432/cmdr", cfg.PostgresURL)
				assert.Equal(t, "http://localhost:8080", cfg.AgentgatewayURL)
			},
		},
		{
			name: "custom values",
			envVars: map[string]string{
				"CMDR_POSTGRES_URL":         "postgres://custom",
				"CMDR_AGENTGATEWAY_URL":    "http://custom:9000",
				"CMDR_API_PORT":            "9090",
				"CMDR_LOG_LEVEL":           "debug",
				"CMDR_WORKER_POOL_SIZE":    "20",
				"CMDR_FREEZETOOLS_PORT":    "8888",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *Config) {
				assert.Equal(t, 9090, cfg.APIPort)
				assert.Equal(t, "debug", cfg.LogLevel)
				assert.Equal(t, 20, cfg.WorkerPoolSize)
				assert.Equal(t, 8888, cfg.FreezeToolsPort)
			},
		},
		{
			name: "missing required postgres url",
			envVars: map[string]string{
				"CMDR_AGENTGATEWAY_URL": "http://localhost:8080",
			},
			wantErr: true,
		},
		{
			name: "missing required agentgateway url",
			envVars: map[string]string{
				"CMDR_POSTGRES_URL": "postgres://localhost:5432/cmdr",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			cfg, err := Load()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: Config{
				APIPort:              8080,
				PostgresURL:          "postgres://localhost:5432/cmdr",
				AgentgatewayURL:     "http://localhost:8080",
				WorkerPoolSize:       10,
				MaxConcurrentReplays: 5,
			},
			wantErr: false,
		},
		{
			name: "invalid port - too low",
			cfg: Config{
				APIPort:              0,
				PostgresURL:          "postgres://localhost:5432/cmdr",
				AgentgatewayURL:     "http://localhost:8080",
				WorkerPoolSize:       10,
				MaxConcurrentReplays: 5,
			},
			wantErr: true,
			errMsg:  "invalid API port",
		},
		{
			name: "invalid port - too high",
			cfg: Config{
				APIPort:              70000,
				PostgresURL:          "postgres://localhost:5432/cmdr",
				AgentgatewayURL:     "http://localhost:8080",
				WorkerPoolSize:       10,
				MaxConcurrentReplays: 5,
			},
			wantErr: true,
			errMsg:  "invalid API port",
		},
		{
			name: "empty postgres url",
			cfg: Config{
				APIPort:              8080,
				PostgresURL:          "",
				AgentgatewayURL:     "http://localhost:8080",
				WorkerPoolSize:       10,
				MaxConcurrentReplays: 5,
			},
			wantErr: true,
			errMsg:  "POSTGRES_URL is required",
		},
		{
			name: "invalid worker pool size",
			cfg: Config{
				APIPort:              8080,
				PostgresURL:          "postgres://localhost:5432/cmdr",
				AgentgatewayURL:     "http://localhost:8080",
				WorkerPoolSize:       0,
				MaxConcurrentReplays: 5,
			},
			wantErr: true,
			errMsg:  "WORKER_POOL_SIZE must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
