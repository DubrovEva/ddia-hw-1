package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ddia/hw1/internal/env"
	"ddia/hw1/internal/grafana"
)

func main() {
	if err := run(); err != nil {
		slog.Error("grafana-init failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	grafanaURL := env.String("GRAFANA_URL", "http://grafana:3000")
	adminUser := env.String("GRAFANA_ADMIN_USER", "admin")
	accountName := env.String("GRAFANA_SA_NAME", "dashgen")
	accountRole := env.String("GRAFANA_SA_ROLE", "Editor")
	tokenFile, err := env.Required("GRAFANA_TOKEN_FILE")
	if err != nil {
		return err
	}
	waitTimeout, err := env.Duration("GRAFANA_WAIT_TIMEOUT", 2*time.Minute)
	if err != nil {
		return err
	}
	adminPassword, err := env.SecretFile("GRAFANA_ADMIN_PASSWORD_FILE")
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := grafana.NewBasicAuth(grafanaURL, adminUser, adminPassword)

	waitCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	slog.Info("waiting for grafana", "url", grafanaURL)
	if err := client.WaitHealthy(waitCtx, 2*time.Second); err != nil {
		return fmt.Errorf("grafana is not healthy: %w", err)
	}

	token, err := client.RotateServiceAccountToken(ctx, accountName, accountRole)
	if err != nil {
		return err
	}
	if err := writeSecretFile(tokenFile, token.Key); err != nil {
		return err
	}
	slog.Info("service account token issued", "service_account", accountName, "role", accountRole, "token_name", token.Name, "file", tokenFile)

	return nil
}

func writeSecretFile(path, secret string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".token-*")
	if err != nil {
		return fmt.Errorf("create token file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.WriteString(secret); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("move token file: %w", err)
	}

	return nil
}
