package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"ddia/hw1/internal/env"
	"ddia/hw1/internal/store"
	"ddia/hw1/pkg/golden"
)

func main() {
	if err := run(); err != nil {
		slog.Error("app failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	addr := env.String("LISTEN_ADDR", ":8080")

	cfg, err := golden.ConfigFromEnv()
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	ins, err := golden.New(cfg, reg)
	if err != nil {
		return fmt.Errorf("init metrics: %w", err)
	}

	flt, err := faultsFromEnv()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	(&api{store: store.New(), faults: flt}).routes(mux, ins)
	mux.Handle("GET /internal/handlers", ins.HandlersHandler())
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr, "pod", cfg.Pod, "datacenter", cfg.Datacenter, "deployment", cfg.Deployment, "fault_rate", flt.rate, "latency_jitter", flt.jitter)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
