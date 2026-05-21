package config

import "testing"

func TestConfigValidateRequiresMandatoryValues(t *testing.T) {
	cfg := Config{}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNormalizePEM(t *testing.T) {
	input := "-----BEGIN RSA PRIVATE KEY-----\\nabc\\n-----END RSA PRIVATE KEY-----"
	got := normalizePEM(input)

	if got == input {
		t.Fatal("expected escaped newlines to be converted")
	}
	if got == "" || got[0] != '-' {
		t.Fatalf("unexpected PEM: %q", got)
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	t.Setenv("DATABASE_HOST", "localhost")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_NAME", "db")
	t.Setenv("DATABASE_USER", "user")
	t.Setenv("DATABASE_PASSWORD", "pa:ss@word")
	t.Setenv("DATABASE_SSLMODE", "")
	t.Setenv("INVITE_CODE", "invite")
	t.Setenv("SESSION_SECRET", "secret")
	t.Setenv("AUTH_CODE_RSA_PRIVATE_KEY", "key")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("AUTH_SERVICE_BASE_URL", "")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("DEFAULT_PROXY_TIMEOUT_SECONDS", "")
	t.Setenv("MAX_PROXY_RETRIES", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ServerAddr != defaultServerAddr {
		t.Fatalf("ServerAddr = %q", cfg.ServerAddr)
	}
	if cfg.AuthServiceBaseURL != defaultAuthServiceURL {
		t.Fatalf("AuthServiceBaseURL = %q", cfg.AuthServiceBaseURL)
	}
	if cfg.Database.SSLMode != defaultDatabaseSSLMode {
		t.Fatalf("Database.SSLMode = %q", cfg.Database.SSLMode)
	}
	if cfg.Database.Password != "pa:ss@word" {
		t.Fatalf("Database.Password = %q", cfg.Database.Password)
	}
}
