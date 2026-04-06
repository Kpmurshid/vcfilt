# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy module files first for layer caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# VERSION is passed from CI via --build-arg VERSION=${GITHUB_REF_NAME}
# Default to "dev" when building locally without a tag
ARG VERSION=dev

RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w -X main.Version=${VERSION}" \
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

# Copy binary to /usr/local/bin so it is on PATH in all runtimes
# (Docker: ENTRYPOINT works with /vcfilt, but Singularity exec needs PATH)
COPY --from=builder /build/vcfilt /usr/local/bin/vcfilt

# Symlink at root for backward compatibility with existing scripts using /vcfilt
COPY --from=builder /build/vcfilt /vcfilt

# Add /usr/local/bin to PATH so `singularity exec vcfilt.sif vcfilt` works
ENV PATH="/usr/local/bin:${PATH}"

ENTRYPOINT ["/usr/local/bin/vcfilt"]
CMD ["--help"]
