# Dockerfile for OpenGSLB Demo Application
# Builds the chaos-enabled demo app

FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source
COPY *.go ./

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o demo-app .

# Runtime image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app

COPY --from=builder /app/demo-app /app/demo-app

# Expose HTTP port
EXPOSE 8080

CMD ["/app/demo-app"]
