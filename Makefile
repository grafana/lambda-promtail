all: clean test build

GOTEST ?= go test
BUMP ?= patch
FILE ?= testdata/albaccesslog.log.gz

build:
	GOOS=linux CGO_ENABLED=0 go build -o ./bootstrap ./...

build-local:
	go build -o ./lambda-promtail-local ./...

test:
	$(GOTEST) ./...

# Run tests with local Loki as the write target (stack must be up via make loki-up)
# Sends testdata/albaccesslog.log.gz to local Loki and verifies output
test-local:
	WRITE_ADDRESS=http://localhost:3100/loki/api/v1/push \
	EXTRA_LABELS=env,local,service,alb \
	OMIT_EXTRA_LABELS_PREFIX=true \
	PRINT_LOG_LINE=true \
	$(GOTEST) ./pkg/ -v -run TestS3

clean:
	rm -f main bootstrap lambda-promtail-local

# Start local Loki + Grafana stack
loki-up:
	docker compose -f dev/docker-compose.yml up -d

# Stop local Loki + Grafana stack
loki-down:
	docker compose -f dev/docker-compose.yml down

# Full local dev loop: start stack then run tests against local Loki
dev: loki-up
	@echo "Loki:    http://localhost:3100"
	@echo "Grafana: http://localhost:3005"
	@echo ""
	@echo "Run 'make test-local' to ingest testdata/albaccesslog.log.gz into local Loki"

.PHONY: release
release:
	@LATEST=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"); \
	MAJOR=$$(echo $$LATEST | sed 's/^v//' | cut -d. -f1); \
	MINOR=$$(echo $$LATEST | sed 's/^v//' | cut -d. -f2); \
	PATCH=$$(echo $$LATEST | sed 's/^v//' | cut -d. -f3); \
	case "$(BUMP)" in \
		major) MAJOR=$$((MAJOR + 1)); MINOR=0; PATCH=0 ;; \
		minor) MINOR=$$((MINOR + 1)); PATCH=0 ;; \
		patch) PATCH=$$((PATCH + 1)) ;; \
		*) echo "Invalid BUMP value: $(BUMP). Use major, minor, or patch." && exit 1 ;; \
	esac; \
	VERSION="v$$MAJOR.$$MINOR.$$PATCH"; \
	echo "Tagging release $$VERSION (was $$LATEST)"; \
	git tag -a $$VERSION -m "Release $$VERSION"; \
	echo "Tag $$VERSION created. Push with: git push origin $$VERSION"
