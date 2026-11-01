# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build server binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bin/qtunnel-server ./cmd/qtunnel-server

# Build client binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bin/qtunnel-client ./cmd/qtunnel-client

# Build diagnose binary
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bin/qtunnel-diagnose ./cmd/qtunnel-diagnose

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates libcap

# Create qtunnel user
RUN addgroup -g 1000 qtunnel && \
    adduser -D -u 1000 -G qtunnel qtunnel

WORKDIR /app

# Copy binaries
COPY --from=builder /app/bin/qtunnel-server /usr/local/bin/
COPY --from=builder /app/bin/qtunnel-client /usr/local/bin/
COPY --from=builder /app/bin/qtunnel-diagnose /usr/local/bin/

# Grant NET_ADMIN capability for TUN device (VPN mode)
RUN setcap cap_net_admin+ep /usr/local/bin/qtunnel-client

# Create directories
RUN mkdir -p /etc/qtunnel /var/log/qtunnel && \
    chown -R qtunnel:qtunnel /etc/qtunnel /var/log/qtunnel

USER qtunnel

# Default command
ENTRYPOINT ["qtunnel-server"]