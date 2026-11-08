.DEFAULT_GOAL := help
.PHONY: help build fmt vet test check race up down logs ps

COMPOSE ?= podman compose
GO ?= go

help:
	@printf '%s\n' \
	  'build  Build the application images' \
	  'fmt    Format Go code' \
	  'vet    Run Go static checks' \
	  'test   Run Go tests' \
	  'check  Run vet and tests' \
	  'race   Run tests with the race detector' \
	  'up     Build and start the Compose services in the background' \
	  'down   Stop and remove the Compose services' \
	  'logs   Follow simulator logs' \
	  'ps     Show Compose service status'

build:
	$(COMPOSE) build

fmt:
	cd backend && $(GO) fmt ./...

vet:
	cd backend && $(GO) vet ./...

test:
	cd backend && $(GO) test ./...

check: vet test

race:
	cd backend && $(GO) test -race ./...

up:
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f simulator

ps:
	$(COMPOSE) ps