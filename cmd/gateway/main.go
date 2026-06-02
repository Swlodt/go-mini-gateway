package main

import (
	"go-mini-gateway/internal/server"
	"log"
)

func main() {
	s, err := server.New(":9090")
	if err != nil {
		log.Fatalf("create gateway failed: %v", err)
	}
	if err := s.Start(); err != nil {
		log.Fatalf("failed to start gateway: %v", err)
	}
}
