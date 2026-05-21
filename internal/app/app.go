package app

import (
	"context"
	"fmt"
	"net/http"

	"simple_gateway_by_codex/internal/config"
	"simple_gateway_by_codex/internal/db"
	"simple_gateway_by_codex/internal/web"
)

// Run starts the gateway service. Feature modules are wired in subsequent steps.
func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := db.RunMigrations(ctx, store.Pool()); err != nil {
		return err
	}

	auth := web.NewBasicAuthForApp(store, cfg.InviteCode, cfg.CookieSecure)
	server := &http.Server{
		Addr:    cfg.ServerAddr,
		Handler: web.NewServerWithServices(cfg.PublicBaseURL, cfg.CookieSecure, auth),
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
