package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServerAddr      = ":8080"
	defaultAuthServiceURL  = "https://auth-service.baichengedu.com"
	defaultDatabaseSSLMode = "disable"
	defaultProxyTimeout    = 30 * time.Second
	defaultMaxRetries      = 3
	dotEnvPath             = ".env"
)

// DatabaseConfig contains PostgreSQL connection settings.
type DatabaseConfig struct {
	Host     string
	Port     int
	Name     string
	User     string
	Password string
	SSLMode  string
}

// Config contains all runtime settings needed by the gateway.
type Config struct {
	ServerAddr          string
	Database            DatabaseConfig
	InviteCode          string
	SessionSecret       string
	AuthServiceBaseURL  string
	RSAPrivateKeyPEM    string
	CookieSecure        bool
	PublicBaseURL       string
	DefaultProxyTimeout time.Duration
	MaxProxyRetries     int
}

// Load reads .env, environment variables, applies defaults, and validates the result.
func Load() (Config, error) {
	_ = loadDotEnv(dotEnvPath)

	rsaPrivateKeyPEM, err := loadRSAPrivateKeyPEM(dotEnvPath)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ServerAddr: getEnv("SERVER_ADDR", defaultServerAddr),
		Database: DatabaseConfig{
			Host:     strings.TrimSpace(os.Getenv("DATABASE_HOST")),
			Name:     strings.TrimSpace(os.Getenv("DATABASE_NAME")),
			User:     strings.TrimSpace(os.Getenv("DATABASE_USER")),
			Password: os.Getenv("DATABASE_PASSWORD"),
			SSLMode:  getEnv("DATABASE_SSLMODE", defaultDatabaseSSLMode),
		},
		InviteCode:          strings.TrimSpace(os.Getenv("INVITE_CODE")),
		SessionSecret:       strings.TrimSpace(os.Getenv("SESSION_SECRET")),
		AuthServiceBaseURL:  strings.TrimRight(getEnv("AUTH_SERVICE_BASE_URL", defaultAuthServiceURL), "/"),
		RSAPrivateKeyPEM:    rsaPrivateKeyPEM,
		PublicBaseURL:       strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")), "/"),
		DefaultProxyTimeout: defaultProxyTimeout,
		MaxProxyRetries:     defaultMaxRetries,
	}

	databasePort, err := parseIntEnv("DATABASE_PORT", 0)
	if err != nil {
		return Config{}, err
	}
	cfg.Database.Port = databasePort

	cookieSecure, err := parseBoolEnv("COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}
	cfg.CookieSecure = cookieSecure

	timeoutSeconds, err := parseIntEnv("DEFAULT_PROXY_TIMEOUT_SECONDS", int(defaultProxyTimeout.Seconds()))
	if err != nil {
		return Config{}, err
	}
	if timeoutSeconds <= 0 {
		return Config{}, errors.New("DEFAULT_PROXY_TIMEOUT_SECONDS must be greater than 0")
	}
	cfg.DefaultProxyTimeout = time.Duration(timeoutSeconds) * time.Second

	maxRetries, err := parseIntEnv("MAX_PROXY_RETRIES", defaultMaxRetries)
	if err != nil {
		return Config{}, err
	}
	if maxRetries < 0 {
		return Config{}, errors.New("MAX_PROXY_RETRIES must be greater than or equal to 0")
	}
	cfg.MaxProxyRetries = maxRetries

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks required settings and URL fields.
func (c Config) Validate() error {
	var missing []string
	if c.Database.Host == "" {
		missing = append(missing, "DATABASE_HOST")
	}
	if c.Database.Port == 0 {
		missing = append(missing, "DATABASE_PORT")
	}
	if c.Database.Name == "" {
		missing = append(missing, "DATABASE_NAME")
	}
	if c.Database.User == "" {
		missing = append(missing, "DATABASE_USER")
	}
	if c.Database.Password == "" {
		missing = append(missing, "DATABASE_PASSWORD")
	}
	if c.InviteCode == "" {
		missing = append(missing, "INVITE_CODE")
	}
	if c.SessionSecret == "" {
		missing = append(missing, "SESSION_SECRET")
	}
	if c.RSAPrivateKeyPEM == "" {
		missing = append(missing, "AUTH_CODE_RSA_PRIVATE_KEY or AUTH_CODE_RSA_PRIVATE_KEY_FILE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return errors.New("DATABASE_PORT must be between 1 and 65535")
	}
	if err := validateDatabaseSSLMode(c.Database.SSLMode); err != nil {
		return err
	}
	if _, err := url.ParseRequestURI(c.AuthServiceBaseURL); err != nil {
		return fmt.Errorf("AUTH_SERVICE_BASE_URL is invalid: %w", err)
	}
	if c.PublicBaseURL != "" {
		if _, err := url.ParseRequestURI(c.PublicBaseURL); err != nil {
			return fmt.Errorf("PUBLIC_BASE_URL is invalid: %w", err)
		}
	}
	return nil
}

func validateDatabaseSSLMode(value string) error {
	switch value {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return nil
	default:
		return fmt.Errorf("DATABASE_SSLMODE is invalid: %s", value)
	}
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		os.Setenv(key, unquoteEnvValue(strings.TrimSpace(value)))
	}
	return scanner.Err()
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}

func parseIntEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func unquoteEnvValue(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		value = value[1 : len(value)-1]
	}
	return strings.ReplaceAll(value, `\n`, "\n")
}

func normalizePEM(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, `\n`, "\n"))
}

func loadRSAPrivateKeyPEM(dotEnvPath string) (string, error) {
	keyFile := strings.TrimSpace(os.Getenv("AUTH_CODE_RSA_PRIVATE_KEY_FILE"))
	if keyFile == "" {
		return normalizePEM(os.Getenv("AUTH_CODE_RSA_PRIVATE_KEY")), nil
	}

	resolvedPath := resolveConfigPath(dotEnvPath, keyFile)
	pemBytes, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("read AUTH_CODE_RSA_PRIVATE_KEY_FILE %q: %w", keyFile, err)
	}
	return normalizePEM(string(pemBytes)), nil
}

func resolveConfigPath(dotEnvPath, configuredPath string) string {
	if filepath.IsAbs(configuredPath) {
		return configuredPath
	}
	return filepath.Join(filepath.Dir(dotEnvPath), configuredPath)
}
