package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ddia/hw1/internal/env"
	"ddia/hw1/internal/loadgen"
)

func main() {
	if err := run(); err != nil {
		slog.Error("loadgen failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	targets := env.List("TARGETS", []string{"http://app-dc1-1:8080", "http://app-dc1-2:8080", "http://app-dc2-1:8080"})
	if len(targets) == 0 {
		return errors.New("env TARGETS must contain at least one url")
	}
	targetRPS, err := env.Float("TARGET_RPS", 60)
	if err != nil {
		return err
	}
	ramp, err := env.Duration("RAMP_DURATION", 2*time.Minute)
	if err != nil {
		return err
	}
	duration, err := env.Duration("LOAD_DURATION", 0)
	if err != nil {
		return err
	}
	workers, err := env.Int("WORKERS", 32)
	if err != nil {
		return err
	}
	badRate, err := env.Float("BAD_REQUEST_RATE", 0.05)
	if err != nil {
		return err
	}
	if badRate < 0 || badRate > 1 {
		return fmt.Errorf("env BAD_REQUEST_RATE must be in [0, 1], got %g", badRate)
	}
	reportInterval, err := env.Duration("REPORT_INTERVAL", 10*time.Second)
	if err != nil {
		return err
	}
	requestTimeout, err := env.Duration("REQUEST_TIMEOUT", 5*time.Second)
	if err != nil {
		return err
	}

	logger := slog.Default()
	runner, err := loadgen.NewRunner(loadgen.Config{
		Ramp:           loadgen.Ramp{Target: targetRPS, Duration: ramp},
		Duration:       duration,
		Workers:        workers,
		ReportInterval: reportInterval,
		RequestTimeout: requestTimeout,
	}, loadgen.NewScenario(targets, badRate, uint64(time.Now().UnixNano())), logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting load", "targets", targets, "target_rps", targetRPS, "ramp", ramp, "duration", duration, "bad_request_rate", badRate)
	stats := runner.Run(ctx)
	logger.Info("load finished",
		"dispatched", stats.Dispatched,
		"ok", stats.OK,
		"4xx", stats.ClientErr,
		"5xx", stats.ServerErr,
		"failed", stats.Failed,
		"dropped", stats.Dropped,
	)

	return nil
}
