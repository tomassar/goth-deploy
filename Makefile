.PHONY: build run dev clean templ deps

# Build the application
build:
	templ generate
	go build -o goth-deploy cmd/server/main.go

# Run the application
run: build
	./goth-deploy

# Development mode with auto-restart (requires air)
dev:
	air

# Generate templ files
templ:
	templ generate

# Install dependencies
deps:
	go mod tidy
	go install github.com/a-h/templ/cmd/templ@latest

# Clean build artifacts
clean:
	rm -f goth-deploy
	find . -name "*_templ.go" -delete

# Setup for development
setup: deps
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@echo "✅ Setup complete! Edit .env with your GitHub OAuth credentials."
	@echo "📝 Get GitHub OAuth credentials at: https://github.com/settings/applications/new"

# Run tests
test:
	go test ./...

# Format code
fmt:
	go fmt ./...
	templ fmt web/templates/ 