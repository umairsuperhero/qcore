.PHONY: build build-hss build-mme build-spgw build-collector build-dashboard build-all test test-short lint clean run run-mme run-spgw run-collector run-dashboard up down web docker-build docker-build-hss docker-build-mme docker-build-spgw docker-build-collector docker-build-dashboard docker-up docker-down coverage

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

build-collector:
	go build $(LDFLAGS) -o bin/qcore-collector ./cmd/qcore-collector

build-dashboard:
	go build $(LDFLAGS) -o bin/qcore-dashboard ./cmd/dashboard

# Rebuild the dashboard's React bundle. Run this after editing files in
# pkg/dashboard/web/src — then re-run `make build-dashboard` to embed.
web:
	cd pkg/dashboard/web && npm install --no-audit --no-fund && npm run build

build-all: build-hss build-mme build-spgw build-collector build-dashboard

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

run-collector: build-collector
	./bin/qcore-collector start

run-dashboard: build-dashboard
	./bin/qcore-dashboard start --config config.example.yaml

# One-command launch (Golden Path step 1). Brings up the full stack:
# postgres, collector, hss, spgw, mme, dashboard. Opens at http://localhost:3000.
up:
	docker compose -f deployments/docker/docker-compose.yml up -d --build
	@echo ""
	@echo "QCore is starting. Open http://localhost:3000 in your browser."
	@echo "Tail logs with: docker compose -f deployments/docker/docker-compose.yml logs -f"
	@echo "Tear down with: make down"

down:
	docker compose -f deployments/docker/docker-compose.yml down

docker-build: docker-build-hss docker-build-mme docker-build-spgw docker-build-collector docker-build-dashboard

docker-build-hss:
	docker build -f deployments/docker/Dockerfile.hss -t qcore-hss:latest .

docker-build-mme:
	docker build -f deployments/docker/Dockerfile.mme -t qcore-mme:latest .

docker-build-spgw:
	docker build -f deployments/docker/Dockerfile.spgw -t qcore-spgw:latest .

docker-build-collector:
	docker build -f deployments/docker/Dockerfile.collector -t qcore-collector:latest .

docker-build-dashboard:
	docker build -f deployments/docker/Dockerfile.dashboard -t qcore-dashboard:latest .

docker-up:
	docker compose -f deployments/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deployments/docker/docker-compose.yml down

coverage: test
	go tool cover -html=coverage.out -o coverage.html
