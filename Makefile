# Goosie Makefile
BINARY_NAME=goosie
BUILD_DIR=bin
GO_FILES=$(shell find . -name '*.go' -not -path "./vendor/*" -not -path "./_v1_old/*")

.PHONY: all build build-small clean test test-race bench gate

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/goosie
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

build-small:
	@echo "Building $(BINARY_NAME) (size optimized)..."
	@mkdir -p $(BUILD_DIR)
	go build -trimpath -ldflags "-s -w -buildid=" -o $(BUILD_DIR)/$(BINARY_NAME)-small ./cmd/goosie
	@echo "Size-optimized build complete: $(BUILD_DIR)/$(BINARY_NAME)-small"

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@go clean

test:
	@echo "Running tests..."
	go test -count=1 ./...

test-race:
	@echo "Running tests with race detector..."
	go test -race ./...

bench:
	@echo "Running benchmarks..."
	go run ./cmd/goosie -bench -frames 2000

gate:
	@echo "Running gate suite..."
	go run ./cmd/goosie -gate -scene checkerboard -frames 600 -out /tmp/gate.json
