package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"simple_gateway_by_codex/internal/authclient"
	"simple_gateway_by_codex/internal/config"
	"simple_gateway_by_codex/internal/cryptoutil"
	"simple_gateway_by_codex/internal/db"
	"simple_gateway_by_codex/internal/web"
)

// Run starts the gateway service. Feature modules are wired in subsequent steps.
func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("gateway.starting",
		"server_addr", cfg.ServerAddr,
		"public_base_url", cfg.PublicBaseURL,
		"auth_service_base_url", cfg.AuthServiceBaseURL,
		"default_proxy_timeout_seconds", int(cfg.DefaultProxyTimeout.Seconds()),
		"max_proxy_retries", cfg.MaxProxyRetries,
		"log_level", cfg.LogLevel,
	)

	store, err := db.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := db.RunMigrations(ctx, store.Pool()); err != nil {
		return err
	}

	cipher, err := cryptoutil.NewAuthorizationCodeCipher(cfg.RSAPrivateKeyPEM)
	if err != nil {
		return err
	}
	authClient := authclient.New(cfg.AuthServiceBaseURL)
	bindingVerifier := web.NewBindingVerifier(authClient)
	auth := web.NewBasicAuthForApp(store, cfg.InviteCode, cfg.CookieSecure, bindingVerifier, cipher)
	server := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: web.NewServerWithDependencies(cfg.PublicBaseURL, cfg.CookieSecure, auth, store, bindingVerifier, cipher, authClient),
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		logger.Info("gateway.stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.DefaultProxyTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func newLogger(level string) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
}
