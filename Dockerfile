# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binaries
RUN CGO_ENABLED=0 go build -o ngrokd ./cmd/ngrokd
RUN CGO_ENABLED=0 go build -o ngrokctl ./cmd/ngrokctl

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    iproute2 \
    iptables

# Create config directory
RUN mkdir -p /etc/ngrokd

# Copy binaries from builder
COPY --from=builder /build/ngrokd /usr/local/bin/ngrokd
COPY --from=builder /build/ngrokctl /usr/local/bin/ngrokctl

# Set permissions
RUN chmod +x /usr/local/bin/ngrokd /usr/local/bin/ngrokctl

# Create default config
COPY config.daemon.yaml /etc/ngrokd/config.yml.example

# Volume for persistent data
VOLUME ["/etc/ngrokd"]

# Expose ports
EXPOSE 8081 9080-9100

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:8081/health || exit 1

# Run as root (required for network interface creation)
USER root

ENTRYPOINT ["/usr/local/bin/ngrokd"]
CMD ["--config=/etc/ngrokd/config.yml"]
