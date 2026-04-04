# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy module files first for layer caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build static binary (CGO disabled for minimal Alpine runtime)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.Version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o vcfilt ./cmd/vcfilt/

# ── Stage 2: Minimal runtime ──────────────────────────────────────────────────
FROM scratch

LABEL org.opencontainers.image.title="vcfilt" \
      org.opencontainers.image.description="High-performance streaming VCF variant filter" \
      org.opencontainers.image.url="https://github.com/Kpmurshid/vcfilt" \
      org.opencontainers.image.source="https://github.com/Kpmurshid/vcfilt" \
      org.opencontainers.image.authors="Kpmurshid" \
      org.opencontainers.image.licenses="MIT"

# Copy CA certs (needed if any HTTPS is ever used)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /build/vcfilt /vcfilt

ENTRYPOINT ["/vcfilt"]
CMD ["--help"]
