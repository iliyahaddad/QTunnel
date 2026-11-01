# QTunnel Roadmap

**Version:** 1.0  
**Date:** 2026-10-25  
**Status:** Living Document

---

## Release Timeline

| Version | Target Date | Focus |
|---------|-------------|-------|
| v0.1.0-alpha | 2026-11-15 | Native QUIC TCP Proxy (M1-M3) |
| v0.2.0-alpha | 2026-12-15 | HTTP/3 Carrier + MASQUE UDP (M4-M5) |
| v0.3.0-alpha | 2027-01-15 | CONNECT-IP + VPN Mode (M5) |
| v0.4.0-beta | 2027-02-15 | Reliability + Hardening (M6) |
| v0.5.0-beta | 2027-03-15 | Quality + Benchmarks (M7) |
| v1.0.0 | 2027-04-15 | Production Release (M8) |

---

## Phase 1: Foundation (v0.1.0-alpha) ✅ IN PROGRESS

### Completed
- [x] Research & Architecture (M1)
- [x] Documentation structure

### In Progress
- [ ] Go module setup
- [ ] Configuration system
- [ ] Authentication framework
- [ ] Session management
- [ ] Frame protocol

### Planned
- [ ] Native QUIC carrier
- [ ] TCP proxy implementation
- [ ] Client/Server CLI
- [ ] Integration tests
- [ ] Basic CI

### Success Criteria
- [ ] Client ↔ Server over QUIC
- [ ] 100 concurrent TCP connections
- [ ] 1GB bidirectional transfer
- [ ] Race detector clean
- [ ] TLS 1.3 enforced

---

## Phase 2: MASQUE Integration (v0.2.0-alpha)

### HTTP/3 Carrier
- [ ] Carrier abstraction layer
- [ ] HTTP/3 client (quic-go http3)
- [ ] HTTP/3 server (quic-go http3)
- [ ] Extended CONNECT support
- [ ] ALPN negotiation (h3)
- [ ] Config-driven transport selection

### CONNECT-UDP (RFC 9298)
- [ ] URI Template parsing/expansion
- [ ] Client: CONNECT-UDP request
- [ ] Server: UDP socket management
- [ ] HTTP Datagrams (Context ID 0)
- [ ] Capsule Protocol for control
- [ ] Destination authorization (DNS + ACL)
- [ ] Interop test with masque-go proxy

### Success Criteria
- [ ] UDP packets proxied bidirectionally
- [ ] Works with masque-go reference proxy
- [ ] Performance within 20% of native QUIC

---

## Phase 3: VPN Mode (v0.3.0-alpha)

### CONNECT-IP (RFC 9484)
- [ ] ADDRESS_ASSIGN capsule
- [ ] ADDRESS_REQUEST capsule
- [ ] ROUTE_ADVERTISEMENT capsule
- [ ] IPv4/IPv6 address assignment
- [ ] Route management

### TUN Device Integration
- [ ] Linux: tun/tap (golang.org/x/sys/unix)
- [ ] Windows: Wintun (wintun-go)
- [ ] macOS: utun (golang.org/x/sys/unix)
- [ ] Cross-platform abstraction

### VPN Features
- [ ] Full-tunnel routing
- [ ] Split-tunnel (allowlist routes)
- [ ] DNS push/intercept
- [ ] Kill switch (leak prevention)
- [ ] MTU handling (IPv6 ≥ 1280)
- [ ] ICMP error signaling

### Success Criteria
- [ ] IP packets routed through TUN
- [ ] Interop with connect-ip-go proxy
- [ ] No DNS leaks
- [ ] Kill switch effective

---

## Phase 4: Reliability (v0.4.0-beta)

### Failure Handling
- [ ] Network chaos testing framework
- [ ] Graceful shutdown (SIGTERM drain)
- [ ] Connection draining
- [ ] Stream recovery design
- [ ] Session resumption design

### Resilience
- [ ] Exponential backoff reconnection
- [ ] Config hot-reload (SIGHUP)
- [ ] Certificate rotation without restart
- [ ] Token rotation without restart
- [ ] Health check endpoints

### Stress Testing
- [ ] 10k concurrent connections
- [ ] Rapid connect/disconnect cycles
- [ ] 24h soak test
- [ ] Memory leak detection
- [ ] Goroutine leak detection

---

## Phase 5: Quality (v0.5.0-beta)

### Testing
- [ ] Unit tests >80% coverage
- [ ] Integration test suite
- [ ] Fuzzing on all parsers
- [ ] Race detector clean
- [ ] Property-based tests (quickcheck)

### Benchmarks
- [ ] Frame encode/decode
- [ ] Session operations
- [ ] QUIC throughput
- [ ] HTTP/3 throughput
- [ ] MASQUE UDP/IP throughput
- [ ] Memory/connection profiling
- [ ] Connection establishment latency

### CI/CD
- [ ] GitHub Actions (Linux, Windows, macOS)
- [ ] Go 1.22, 1.23 matrix
- [ ] golangci-lint
- [ ] govulncheck
- [ ] gosec
- [ ] Benchmark tracking
- [ ] Release automation (goreleaser)

---

## Phase 6: Release (v1.0.0)

### Documentation
- [ ] Complete README with examples
- [ ] Architecture diagrams
- [ ] Configuration reference
- [ ] Deployment guides (Docker, systemd, Kubernetes)
- [ ] API documentation (if library)
- [ ] Man pages

### Security
- [ ] Security audit (internal)
- [ ] Dependency audit
- [ ] Threat model review
- [ ] Penetration test (if budget)
- [ ] Signed releases (cosign)

### Compatibility
- [ ] Interop matrix published
- [ ] Known limitations documented
- [ ] Migration guide v0.x → v1.0

---

## Post-v1.0 Ideas

### v1.1 - HTTP/2 Fallback
- [ ] Capsule Protocol over HTTP/2
- [ ] Custom framing for TCP proxy
- [ ] ALPN negotiation (h2)
- [ ] Documented as QTunnel-specific

### v1.2 - Session Resumption
- [ ] Ticket-based resumption
- [ ] Stream state reconstruction
- [ ] Sequence numbers for in-flight data
- [ ] 0-RTT with anti-replay

### v1.3 - Multipath
- [ ] QUIC Multipath (draft-ietf-quic-multipath)
- [ ] Multiple paths per connection
- [ ] Path migration API

### v1.4 - Advanced Features
- [ ] Traffic padding (timing analysis resistance)
- [ ] Constant-rate mode
- [ ] Post-quantum KEM (when standardized)
- [ ] Hardware key storage (TPM/HSM)
- [ ] ZTNA integration

### v2.0 - Architecture Evolution
- [ ] Plugin architecture for carriers
- [ ] WebAssembly for custom logic
- [ ] Control plane separation
- [ ] Multi-tenancy

---

## Prioritization Framework

| Priority | Criteria |
|----------|----------|
| P0 (Blocker) | Security, data loss, crash |
| P1 (High) | Core feature broken, perf regression >20% |
| P2 (Medium) | Feature request, minor bug, doc gap |
| P3 (Low) | Nice-to-have, refactoring, cleanup |

---

## Resource Requirements

| Role | Phase 1 | Phase 2 | Phase 3 | Phase 4+ |
|------|---------|---------|---------|----------|
| Backend Engineer | 1 | 1-2 | 1-2 | 1 |
| Security Review | - | 0.5 | 0.5 | 0.5 |
| DevOps/CI | 0.5 | 0.5 | 0.5 | 1 |
| Technical Writer | - | - | 0.5 | 1 |

---

## Risk Mitigation

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| quic-go API breaking change | Medium | High | Pin version; vendor if needed |
| MASQUE HTTP/2 not implementable | High | Medium | Document limitation; defer |
| TUN platform complexity | High | High | Linux first; stub others |
| Interop testing infrastructure | Medium | Medium | Use upstream test servers |
| Performance targets missed | Low | Medium | Early benchmarking |

---

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-10-25 | Native QUIC first | Simplest working foundation |
| 2026-10-25 | MASQUE via quic-go libs | Standards-based, maintained |
| 2026-10-25 | HTTP/2 fallback deferred | No library support; custom work |
| 2026-10-25 | MIT License | Compatible with all deps |

---

## Community

- **Issues:** GitHub Issues (bug reports, feature requests)
- **Discussions:** GitHub Discussions (questions, ideas)
- **Security:** security@qtunnel.example.com
- **Contributing:** See CONTRIBUTING.md

---

## Metrics for Success

| Metric | v0.1 | v0.3 | v1.0 |
|--------|------|------|------|
| GitHub Stars | 50 | 200 | 1000 |
| Contributors | 1 | 3 | 10 |
| Production Users | 0 | 5 | 50 |
| Interop Verified | QUIC | QUIC+MASQUE | All |
| Security Issues | 0 | 0 | 0 |
| Coverage | 80% | 85% | 90% |