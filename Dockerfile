# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /opengslb ./cmd/opengslb

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user. The UID is fixed (it is the one Alpine assigned before)
# so that the owner of bind-mounted directories can be set on the host.
RUN adduser -D -g '' -u 1000 appuser

# Data directory: the overwatch store (overwatch.data_dir) and the agent
# certificate and key (agent.identity.cert_path, key_path) default to
# /var/lib/opengslb. It must exist and belong to the runtime user, who cannot
# create it under /var/lib. A named or anonymous volume mounted here starts as
# a copy of this directory, with its owner.
RUN mkdir -p /var/lib/opengslb \
    && chown appuser:appuser /var/lib/opengslb \
    && chmod 0750 /var/lib/opengslb

WORKDIR /app

# Copy binary from builder
COPY --from=builder /opengslb .

# Copy config directory if needed
# COPY --from=builder /app/configs ./configs

# Use non-root user
USER appuser

# Persistent data. Mount a named volume here to keep it when the container is
# replaced: -v opengslb-data:/var/lib/opengslb
VOLUME ["/var/lib/opengslb"]

# Expose ports: DNS (TCP/UDP) and metrics
EXPOSE 53/udp 53/tcp 9090

# Run the binary
ENTRYPOINT ["./opengslb"]