VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X 'github.com/Yachiiiiyo/xLyra-cli/internal/version.Version=$(VERSION)' \
	-X 'github.com/Yachiiiiyo/xLyra-cli/internal/version.Commit=$(COMMIT)' \
	-X 'github.com/Yachiiiiyo/xLyra-cli/internal/version.BuildTime=$(BUILD_TIME)'

.PHONY: build test vet release clean

build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/xlyra ./cmd/xlyra

vet:
	go vet ./...

test:
	go test -race -v ./...

release:
	VERSION="$(VERSION)" COMMIT="$(COMMIT)" BUILD_TIME="$(BUILD_TIME)" ./scripts/build-release.sh

clean:
	rm -rf bin dist
