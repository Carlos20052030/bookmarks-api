package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

const (
	defaultServerPort = 8080
	defaultLogLevel   = "info"
	minJWTSecretLen   = 32
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	JWT      JWTConfig
	LogLevel string
}

type ServerConfig struct {
	Port int
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

// DSN returns the Postgres connection string for database/sql.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		d.User, d.Password, d.Host, d.Port, d.Name,
	)
}

type JWTConfig struct {
	Secret []byte
}

// Load reads configuration from environment variables and validates it.
// It fails fast on the first invalid value so the caller can exit at startup.
func Load() (*Config, error) {
	cfg := &Config{
		Server:   ServerConfig{Port: defaultServerPort},
		LogLevel: defaultLogLevel,
	}

	if v := os.Getenv("SERVER_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("SERVER_PORT is not a valid integer: %w", err)
		}
		cfg.Server.Port = p
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	cfg.Database.Host = os.Getenv("POSTGRES_HOST")
	cfg.Database.User = os.Getenv("POSTGRES_USER")
	cfg.Database.Password = os.Getenv("POSTGRES_PASSWORD")
	cfg.Database.Name = os.Getenv("POSTGRES_DB")

	required := []struct {
		name  string
		value string
	}{
		{"POSTGRES_HOST", cfg.Database.Host},
		{"POSTGRES_USER", cfg.Database.User},
		{"POSTGRES_PASSWORD", cfg.Database.Password},
		{"POSTGRES_DB", cfg.Database.Name},
	}
	for _, r := range required {
		if r.value == "" {
			return nil, fmt.Errorf("%s is required", r.name)
		}
	}

	portStr := os.Getenv("POSTGRES_PORT")
	if portStr == "" {
		return nil, errors.New("POSTGRES_PORT is required")
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("POSTGRES_PORT is not a valid integer: %w", err)
	}
	cfg.Database.Port = p

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	if len(secret) < minJWTSecretLen {
		return nil, fmt.Errorf("JWT_SECRET must be at least %d bytes", minJWTSecretLen)
	}
	cfg.JWT.Secret = []byte(secret)

	return cfg, nil
}
