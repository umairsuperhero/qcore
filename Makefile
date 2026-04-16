.PHONY: build build-hss build-mme build-spgw build-all test test-short lint clean run run-mme run-spgw docker-build docker-build-hss docker-build-mme docker-build-spgw docker-up docker-down coverage

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(DATE)"

build: build-all

build-hss:
	go build $(LDFLAGS) -o bin/qcore-hss ./cmd/hss

build-mme:
	go build $(LDFLAGS) -o bin/qcore-mme ./cmd/mme

build-spgw:
	go build $(LDFLAGS) -o bin/qcore-spgw ./cmd/spgw

build-all: build-hss build-mme build-spgw

test:
	go test -v -race -coverprofile=coverage.out ./...

test-short:
	go test -v -short ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ coverage.out coverage.html

run: build-hss
	./bin/qcore-hss start --config config.example.yaml

run-mme: build-mme
	./bin/qcore-mme start --config config.example.yaml

run-spgw: build-spgw
	./bin/qcore-spgw start --config config.example.yaml

docker-build: docker-build-hss docker-build-mme docker-build-spgw

docker-build-hss:
	docker build -f deployments/docker/Dockerfile.hss -t qcore-hss:latest .

docker-build-mme:
	docker build -f deployments/docker/Dockerfile.mme -t qcore-mme:latest .

docker-build-spgw:
	docker build -f deployments/docker/Dockerfile.spgw -t qcore-spgw:latest .

docker-up:
	docker compose -f deployments/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker/docker-compose.yml down

coverage: test
	go tool cover -html=coverage.out -o coverage.html
