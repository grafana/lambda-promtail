all: clean test build

GOTEST ?= go test
BUMP ?= patch

build:
	GOOS=linux CGO_ENABLED=0 go build -o ./main ./...

test:
	$(GOTEST) ./...

clean:
	rm -f main

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
