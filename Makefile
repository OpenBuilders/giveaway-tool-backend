SHELL := /bin/bash
APP_NAME := giveaway-backend
GO ?= go
GOMOD := $(shell go env GOMOD)
GOFILES := $(shell find . -name "*.go" -not -path "*/vendor/*")

.PHONY: tidy build run test lint goose-up goose-down goose-status migrate-create

tidy:
	$(GO) mod tidy

build:
	$(GO) build -o bin/$(APP_NAME) ./cmd/api

run:
	$(GO) run ./cmd/api

test:
	$(GO) test ./...

lint:
	golangci-lint run || true

goose-up:
	goose -dir ./migrations postgres "$$DATABASE_URL" up

goose-down:
	goose -dir ./migrations postgres "$$DATABASE_URL" down

goose-status:
	goose -dir ./migrations status

migrate-create:
	@test -n "$(name)" || (echo "Usage: make migrate-create name=add_users" && exit 1)
	goose -dir ./migrations create $(name) sql
