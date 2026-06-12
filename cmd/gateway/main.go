package main

import (
	"context"
	"flag"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/server"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// 记录所有阻塞事件，开销较高
	//runtime.SetBlockProfileRate(1)
	// 记录所有 mutex 竞争事件，开销较高
	//runtime.SetMutexProfileFraction(1)

	configPath := flag.String("config", "configs/gateway.json", "gateway config file path")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	shutdownTimeout, err := cfg.ShutdownTimeoutDuration()
	if err != nil {
		log.Fatalf("invalid shutdown timeout: %v", err)
	}

	s, err := server.New(cfg)
	if err != nil {
		log.Fatalf("create gateway failed: %v", err)
	}

	errCh := make(chan error, 1)

	go func() {
		errCh <- s.Start()
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("failed to start gateway: %v", err)
		}
		return
	case <-signalCtx.Done():
		stop()
		log.Printf("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := s.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)

		if closeErr := s.Close(); closeErr != nil {
			log.Printf("force close failed: %v", closeErr)
		}
	}

	if err := <-errCh; err != nil {
		log.Fatalf("gateway stopped with error: %v", err)
	}

	log.Printf("gateway stopped")

}
