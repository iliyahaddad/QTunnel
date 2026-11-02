# QTunnel

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://golang.org)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)]()
[![Security](https://img.shields.io/badge/security-audited-blue.svg)](SECURITY.md)

A modular, multi-transport network tunnel with QUIC, HTTP/3, and MASQUE support.

## Overview

QTunnel is a network tunneling engine designed with a clear separation between session logic and transport carriers. It supports multiple transport protocols:

| Transport | Status | Use Case |
|-----------|--------|----------|
| **Native QUIC** | ✅ v0.1 (MVP) | TCP proxy, simple deployment |
| **HTTP/3 + MASQUE** | 🟡 v0.2 | UDP proxy (CONNECT-UDP), VPN (CONNECT-IP) |
| **HTTP/2 Fallback** | 📋 Planned | Restricted networks (custom protocol) |

## Features

### Core (v0.1)
- 🔒 **TLS 1.3 only** - No downgrade, no weak ciphers
- 🔐 **Token authentication** - 256-bit entropy, constant-time compare, hot reload
- 📦 **TCP Proxy** - Bidirectional streaming over QUIC streams
- ⚡ **Connection migration** - Native QUIC mobility support
- 🛡️ **Resource limits** - Connections, streams, buffers, timeouts
- 📊 **Observability** - Prometheus metrics, structured logging, health checks
- 🧪 **Well-tested** - Unit, integration, fuzz, race detector

### MASQUE (v0.2+)
- 🌐 **CONNECT-UDP** (RFC 9298) - Standards-based UDP proxying
- 🔗 **CONNECT-IP** (RFC 9484) - Full IP-layer VPN
- 🖥️ **TUN Integration** - Linux, Windows (Wintun), macOS
- 🔀 **Split tunneling** - Route selection, DNS handling
- 🛑 **Kill switch** - Leak prevention

## Quick Start

### Prerequisites
- Go 1.23+ (for building)
- TLS certificate (Let's Encrypt recommended for production)

### Build from Source
```bash
git clone https://github.com/iliyahaddad/qtunnel
cd qtunnel
make build
make certs  # Generates test certificates
```

### Configuration

**Server** (`configs/server.yaml`):
```yaml
role: "server"
listen:
  addr: ":4433"
tls:
  cert_file: "server.crt"
  key_file: "server.key"
auth:
  tokens: ["base64-encoded-32-byte-token"]
proxy:
  mode: "tcp"
  allowlist: ["*"]
  denylist: ["10.0.0.0/8", "127.0.0.0/8", "::1"]
limits:
  max_connections: 1000
  max_streams_per_conn: 100
```

**Client** (`configs/client.yaml`):
```yaml
role: "client"
server:
  addr: "your-server.example.com:4433"
  sni: "your-server.example.com"
auth:
  token: "base64-encoded-32-byte-token"
proxy:
  mode: "tcp"
  listen: "127.0.0.1:8080"
```

### Run

```bash
# Terminal 1: Server
./bin/qtunnel-server -c configs/server.yaml

# Terminal 2: Client
./bin/qtunnel-client -c configs/client.yaml

# Terminal 3: Test
curl -v http://httpbin.org/get --proxy http://127.0.0.1:8080
```

### Generate Token
```bash
./bin/qtunnel-diagnose secret
# Output: base64-encoded 32-byte token
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                        │
│  TCP Proxy  │  SOCKS5 (planned)  │  VPN/TUN (planned)       │
├─────────────────────────────────────────────────────────────┤
│                    Session Layer                            │
│  Auth  │  Stream Mux  │  Flow Control  │  Lifecycle         │
├─────────────────────────────────────────────────────────────┤
│                    Frame Layer                              │
│  CONNECT │ DATA │ CLOSE │ PING │ AUTH │ WINDOW_UPDATE       │
├─────────────────────────────────────────────────────────────┤
│                    Carrier Layer                            │
│  Native QUIC  │  HTTP/3 (MASQUE)  │  HTTP/2 (planned)       │
└─────────────────────────────────────────────────────────────┘
```

## Documentation

| Document | Description |
|----------|-------------|
| [THREAT-MODEL.md](docs/THREAT-MODEL.md) | Comprehensive threat model |
| [PROTOCOL.md](docs/PROTOCOL.md) | Wire protocol specification |
| [SECURITY.md](docs/SECURITY.md) | Security policy |
| [TESTING.md](docs/TESTING.md) | Testing guide |
| [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) | Common issues |
| [ROADMAP.md](docs/ROADMAP.md) | Future plans |

## Security

- **No custom crypto** - Uses Go stdlib TLS 1.3 and quic-go
- **Certificate verification mandatory** - Cannot be disabled in production config
- **Token authentication** - 256-bit entropy, constant-time comparison
- **Resource limits** - Prevents DoS via connection/stream/buffer bounds
- **Fuzzing** - All parsers fuzzed in CI
- **Race detector** - All tests run with `-race` in CI

See [SECURITY.md](docs/SECURITY.md) for vulnerability reporting and secure configuration guidelines.

## Testing

```bash
# Unit tests
make test

# With race detector
make test-race

# With coverage
make test-cover

# Fuzzing (30s per target)
make test-fuzz

# Benchmarks
make test-bench

# Integration tests
make test-integration
```

## Building for Production

```bash
# Cross-compile
GOOS=linux GOARCH=amd64 make build
GOOS=windows GOARCH=amd64 make build
GOOS=darwin GOARCH=arm64 make build

# With version info
make build VERSION=v1.0.0
```

## Deployment

### Docker
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN make build

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/bin/qtunnel-server /usr/local/bin/
COPY --from=builder /app/bin/qtunnel-client /usr/local/bin/
ENTRYPOINT ["qtunnel-server"]
```

### systemd
```ini
[Unit]
Description=QTunnel Server
After=network.target

[Service]
Type=simple
User=qtunnel
ExecStart=/usr/local/bin/qtunnel-server -c /etc/qtunnel/server.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

## Configuration Reference

See [configs/client.example.yaml](configs/client.example.yaml) and [configs/server.example.yaml](configs/server.example.yaml) for complete examples.

### Key Options

| Option | Description | Default |
|--------|-------------|---------|
| `transport.type` | Carrier: `quic`, `http3` | `quic` |
| `transport.quic.max_idle_timeout` | Idle timeout | `30s` |
| `transport.quic.keep_alive` | Keep-alive interval | `10s` |
| `limits.max_connections` | Max concurrent connections | `1000` |
| `limits.max_streams_per_conn` | Max streams per connection | `100` |
| `limits.stream_buffer_size` | Per-stream receive buffer | `1MB` |
| `proxy.allowlist` | Destination allowlist | `["*"]` |
| `proxy.denylist` | Destination denylist | RFC1918, localhost |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development workflow, coding standards, and PR process.

## License

see [LICENSE](LICENSE) for details.

## Acknowledgments

- [quic-go](https://github.com/quic-go/quic-go) - QUIC/HTTP/3 implementation
- [masque-go](https://github.com/quic-go/masque-go) - CONNECT-UDP reference
- [connect-ip-go](https://github.com/quic-go/connect-ip-go) - CONNECT-IP reference
- [IETF MASQUE WG](https://datatracker.ietf.org/wg/masque/) - Standards development

## Status

**Current: v0.1.0-alpha (Milestone 2 - Foundation)**

See [ROADMAP.md](docs/ROADMAP.md) for release timeline.
