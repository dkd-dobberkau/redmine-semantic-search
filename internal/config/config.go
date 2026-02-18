// Package config loads and validates application configuration from a YAML file
// with environment variable overrides. Missing required fields cause an immediate
// exit with a clear error listing all missing values.
package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Config holds all application configuration parameters.
type Config struct {
	// RedmineURL is the base URL of the Redmine instance (e.g. https://redmine.example.com).
	// Required. Environment variable: REDMINE_URL
	RedmineURL string `mapstructure:"redmine_url" validate:"required,url"`

	// RedmineAPIKey is the Redmine REST API key used for authentication.
	// Required. Environment variable: REDMINE_API_KEY
	RedmineAPIKey string `mapstructure:"redmine_api_key" validate:"required"`

	// QdrantHost is the hostname or IP address of the Qdrant gRPC endpoint.
	// Required. Environment variable: QDRANT_HOST
	QdrantHost string `mapstructure:"qdrant_host" validate:"required"`

	// QdrantPort is the gRPC port of the Qdrant service. Default: 6334.
	// Environment variable: QDRANT_PORT
	QdrantPort int `mapstructure:"qdrant_port"`

	// EmbeddingURL is the base URL of the text embeddings inference service.
	// Required. Environment variable: EMBEDDING_URL
	EmbeddingURL string `mapstructure:"embedding_url" validate:"required,url"`
}

// Load reads configuration from config.yml (if present) and environment variables,
// validates all required fields, and returns a populated Config or an error listing
// every missing or invalid field.
//
// Environment variable override rules:
//   - Each config key maps to an env var by uppercasing and replacing dots with underscores.
//   - Example: redmine_api_key → REDMINE_API_KEY
//
// Config file is optional. If absent, all values must be provided via environment variables.
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// Set defaults for all fields — critical for AutomaticEnv + Unmarshal to work
	// in viper v1.19+ (see: github.com/spf13/viper/issues/1895).
	// Without SetDefault(), AutomaticEnv() silently ignores env vars during Unmarshal.
	viper.SetDefault("redmine_url", "")
	viper.SetDefault("redmine_api_key", "")
	viper.SetDefault("qdrant_host", "")
	viper.SetDefault("qdrant_port", 6334)
	viper.SetDefault("embedding_url", "")

	// Map env vars: QDRANT_HOST → qdrant_host, REDMINE_API_KEY → redmine_api_key, etc.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config file: %w", err)
		}
		// No config file found — env-only configuration is valid.
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(&cfg); err != nil {
		var errs validator.ValidationErrors
		if ok := errorsAs(err, &errs); ok {
			return nil, formatValidationErrors(errs)
		}
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

// errorsAs is a helper to unwrap validator.ValidationErrors from an error.
func errorsAs(err error, target *validator.ValidationErrors) bool {
	if ve, ok := err.(validator.ValidationErrors); ok {
		*target = ve
		return true
	}
	return false
}

// formatValidationErrors returns a human-readable error listing all validation failures.
func formatValidationErrors(errs validator.ValidationErrors) error {
	var sb strings.Builder
	sb.WriteString("config validation failed — missing or invalid fields:\n")
	for _, e := range errs {
		field := e.Field()
		tag := e.Tag()
		switch tag {
		case "required":
			sb.WriteString(fmt.Sprintf("  - %s: required but not set\n", field))
		case "url":
			sb.WriteString(fmt.Sprintf("  - %s: must be a valid URL (got: %q)\n", field, e.Value()))
		default:
			sb.WriteString(fmt.Sprintf("  - %s: failed validation %q\n", field, tag))
		}
	}
	return fmt.Errorf("%s", sb.String())
}
