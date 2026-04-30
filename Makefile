.PHONY: all build run worker migrate-up migrate-down swag test lint clean

# Binary names
API_BINARY=bin/api
WORKER_BINARY=bin/worker

# Build both binaries
build:
	@echo "Building API..."
	@go build -o $(API_BINARY) ./cmd/api
	@echo "Building Worker..."
	@go build -o $(WORKER_BINARY) ./cmd/worker

# Run API server
run:
	@go run ./cmd/api/main.go

# Run Worker
worker:
	@go run ./cmd/worker/main.go

# Run both with air (hot reload) - install: go install github.com/air-verse/air@latest
dev:
	@air -c .air.api.toml

# Database migrations
migrate-up:
	@migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	@migrate -path migrations -database "$(DATABASE_URL)" down

migrate-version:
	@migrate -path migrations -database "$(DATABASE_URL)" version

# Generate swagger docs
swag:
	@swag init -g cmd/api/main.go -o docs

# Run tests
test:
	@go test ./... -v -race -cover

test-short:
	@go test ./... -short

# Lint - install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
lint:
	@golangci-lint run ./...

# Clean build artifacts
clean:
	@rm -rf bin/
	@rm -rf docs/swagger*

# Install tools
tools:
	@go install github.com/air-verse/air@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/swaggo/swag/cmd/swag@latest
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Docker
docker-up:
	@docker compose up -d

docker-down:
	@docker compose down

docker-logs:
	@docker compose logs -f

# Tidy modules
tidy:
	@go mod tidy
