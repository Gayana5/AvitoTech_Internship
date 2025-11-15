.PHONY: build run test clean docker-build docker-up docker-down

# Build the application
build:
	go build -o bin/server ./cmd/server

# Run the application locally (requires PostgreSQL running)
run:
	go run ./cmd/server

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Build Docker image
docker-build:
	docker-compose build

# Start services with Docker Compose
docker-up:
	docker-compose up -d

# Stop services
docker-down:
	docker-compose down

# View logs
docker-logs:
	docker-compose logs -f app

# Run migrations manually (if needed)
migrate:
	go run ./cmd/server

# Install dependencies
deps:
	go mod download
	go mod tidy

