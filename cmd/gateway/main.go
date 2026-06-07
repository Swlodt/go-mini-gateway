package main

import (
	"flag"
	"go-mini-gateway/internal/config"
	"go-mini-gateway/internal/server"
	"log"
)

func main() {
	configPath := flag.String("config", "configs/gateway.json", "gateway config file path")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}
	s, err := server.New(cfg)
	if err != nil {
		log.Fatalf("create gateway failed: %v", err)
	}
	if err := s.Start(); err != nil {
		log.Fatalf("failed to start gateway: %v", err)
	}
}
