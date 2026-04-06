# vcfilt Makefile
# Usage:
#   make          - build binary with version from git tag
#   make install  - install to /usr/local/bin
#   make test     - run all tests
#   make bench    - run benchmarks
#   make clean    - remove build artifacts

BINARY  := vcfilt
CMD     := ./cmd/vcfilt/

# Derive version from git tag (e.g. v1.0.0); fall back to "dev" if no tag
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: all build install test bench clean

all: build

build:
	@echo "Building $(BINARY) $(VERSION)..."
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) $(CMD)
	@echo "Done: ./$(BINARY)"
	@./$(BINARY) --version

install: build
	@echo "Installing to /usr/local/bin/$(BINARY)..."
	install -m 755 $(BINARY) /usr/local/bin/$(BINARY)

test:
	go test ./...

bench:
	go test -bench=. -benchmem ./internal/parser/ ./internal/filter/

clean:
	rm -f $(BINARY)
