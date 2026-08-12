.PHONY: build run test

BIN_DIR := bin
BINARY := $(BIN_DIR)/company-service

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/company-service

run:
	go run ./cmd/company-service

test:
	go test ./...
