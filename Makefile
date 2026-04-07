.PHONY: build test test-short lint clean run docker-build docker-up docker-down coverage

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)"

build:
	go build $(LDFLAGS) -o bin/qcore-hss ./cmd/hss

test:
	go test -v -race -coverprofile=coverage.out ./...

test-short:
	go test -v -short ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ coverage.out coverage.html

run: build
	./bin/qcore-hss start --config config.example.yaml

docker-build:
	docker build -f deployments/docker/Dockerfile.hss -t qcore-hss:latest .

docker-up:
	docker compose -f deployments/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker/docker-compose.yml down

coverage: test
	go tool cover -html=coverage.out -o coverage.html
