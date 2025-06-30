.PHONY: help install dev build clean generate test format

# Default target
help:
	@echo "GoTH Deployer - Available commands:"
	@echo "  install    Install dependencies and tools"
	@echo "  dev        Run in development mode with live reload"
	@echo "  build      Build the application"
	@echo "  generate   Generate templ files"
	@echo "  clean      Clean build artifacts"
	@echo "  test       Run tests"
	@echo "  format     Format code"

# Install dependencies and tools
install:
	@echo "Installing Go dependencies..."
	go mod download
	@echo "Installing templ CLI..."
	go install github.com/a-h/templ/cmd/templ@latest
	@echo "Creating necessary directories..."
	mkdir -p data deployments/logs web/static
	@echo "Installation complete!"

# Generate templ files
generate:
	@echo "Generating templ files..."
	templ generate

# Run in development mode
dev: generate
	@echo "Starting development server..."
	go run cmd/server/main.go

# Build the application
build: generate
	@echo "Building application..."
	go build -o bin/deployer cmd/server/main.go

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf web/templates/*_templ.go

# Run tests
test:
	@echo "Running tests..."
	go test ./...

# Format code
format:
	@echo "Formatting code..."
	go fmt ./...
	templ fmt web/templates/ 