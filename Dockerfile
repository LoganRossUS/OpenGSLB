# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /opengslb ./cmd/opengslb

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /opengslb .

# Copy config directory if needed
# COPY --from=builder /app/configs ./configs

# Use non-root user
USER appuser

# Expose ports: DNS (TCP/UDP) and metrics
EXPOSE 53/udp 53/tcp 9090

# Run the binary
ENTRYPOINT ["./opengslb"]