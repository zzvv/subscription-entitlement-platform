package main

import (
	"example.com/subscription-entitlement-platform/internal/application"
	"example.com/subscription-entitlement-platform/internal/config"
	"example.com/subscription-entitlement-platform/internal/repository"
	"example.com/subscription-entitlement-platform/internal/transport"
	"log"
	"net/http"
)

func main() {
	cfg := config.Load()
	service := application.NewService(repository.NewStore())
	log.Fatal(http.ListenAndServe(cfg.Address, transport.Routes(transport.New(service))))
}
