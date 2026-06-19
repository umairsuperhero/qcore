.PHONY: build build-hss build-mme build-spgw build-collector build-dashboard build-all test test-short lint clean run run-mme run-spgw run-collector run-dashboard up up-5g up-ai down web measure capture-real-ran adoption-report verify-fast verify-full verify-t10 fact-check docker-build docker-build-hss docker-build-mme docker-build-spgw docker-build-collector docker-build-dashboard docker-up docker-down coverage

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

measure:
	./scripts/measure-ttfc-ttrc.sh

capture-real-ran:
	./scripts/ci/real-ran-capture.sh

adoption-report:
	python3 scripts/adoption-report.py --output artifacts/adoption/report.md --json-output artifacts/adoption/report.json

adoption-draft:
	python3 scripts/adoption/draft-weekly-update.py

verify-fast:
	./scripts/verify-fast.sh

verify-full:
	./scripts/verify-full.sh

verify-t10:
	./scripts/verify-t10-github.sh

fact-check:
	./scripts/fact-check-pr.sh

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

# Opt-in 5G SA stack (NRF, UDR, UDM, AUSF, AMF, SMF, UPF + UERANSIM). These
# services live behind the "5g" Compose profile, so the default `make up` does
# not start them. Note: real NGAP-over-SCTP + UERANSIM TUN needs a Linux host;
# on macOS the 5G control plane runs in SCTP tcp-fallback mode.
up-5g:
	COMPOSE_PROFILES=5g docker compose -f deployments/docker/docker-compose.yml up -d --build
	@echo ""
	@echo "QCore 4G + 5G stack starting. Dashboard: http://localhost:3000"
	@echo "Tear down everything with: make down"

# Opt-in offline Diagnostic AI (charter §9.3). Brings up the qcore-slm sidecar
# (a ~1 GB small instruct model served over an OpenAI-compatible API) so the AI
# explains failures with no cloud account and no API key. The dashboard already
# defaults to the local provider, so this just makes the model reachable. The
# first build downloads the model weights (~1 GB), cached thereafter.
up-ai:
	COMPOSE_PROFILES=ai docker compose -f deployments/docker/docker-compose.yml up -d --build
	@echo ""
	@echo "QCore + offline Diagnostic AI starting. Dashboard: http://localhost:3000"
	@echo "The SLM serves at http://localhost:8088/v1 (first start pulls ~1 GB)."
	@echo "Tear down everything with: make down"

down:
	COMPOSE_PROFILES=5g,ai docker compose -f deployments/docker/docker-compose.yml down

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
