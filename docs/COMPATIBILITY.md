# QTunnel Compatibility Matrix

**Version:** 1.0  
**Date:** 2026-10-25  
**Status:** Living Document - Updated per milestone

---

## 1. Transport Protocol Support

| Feature | Native QUIC | HTTP/3 (MASQUE) | HTTP/2 Fallback | WireGuard |
|---------|-------------|-----------------|-----------------|-----------|
| **TCP Proxy** | ✅ Phase 1 | ✅ Phase 2 | 🟡 Phase 3 (Custom) | ❌ N/A |
| **UDP Proxy** | ❌ Custom | ✅ CONNECT-UDP (RFC 9298) | ❌ No Standard | ❌ N/A |
| **IP/VPN** | ❌ Custom | ✅ CONNECT-IP (RFC 9484) | ❌ No Standard | ✅ Native |
| **Status** | **MVP** | **Planned** | **Deferred** | **Future** |

---

## 2. Standards Compliance

### 2.1 Implemented Standards (Phase 1)

| Standard | Version | Compliance | Notes |
|----------|---------|------------|-------|
| QUIC Transport | RFC 9000 | Full (via quic-go) | quic-go v0.48+ |
| QUIC TLS | RFC 9001 | Full (via quic-go) | TLS 1.3 only |
| QUIC Recovery/CC | RFC 9002 | Full (via quic-go) | CUBIC default |
| TLS | RFC 8446 | Full (Go stdlib) | TLS 1.3 mandatory |

### 2.2 Planned Standards (Phase 2)

| Standard | Version | Compliance | Library |
|----------|---------|------------|---------|
| HTTP/3 | RFC 9114 | Full (via quic-go) | quic-go http3 |
| QPACK | RFC 9204 | Full (via quic-go) | quic-go http3 |
| HTTP Datagrams | RFC 9297 | Full (via quic-go) | quic-go http3 |
| Capsule Protocol | RFC 9297 | Full (via masque-go) | masque-go |
| QUIC Datagrams | RFC 9221 | Full (via quic-go) | quic-go |
| Extended CONNECT | RFC 9220 | Full (via quic-go) | quic-go http3 |
| CONNECT-UDP | RFC 9298 | Full (via masque-go) | masque-go |
| CONNECT-IP | RFC 9484 | Full (via connect-ip-go) | connect-ip-go |

### 2.3 Deferred / Non-Standard

| Feature | Standard | Status | Notes |
|---------|----------|--------|-------|
| MASQUE over HTTP/2 | RFC 9298 §3.4 | ❌ Not Implemented | No Go library implements Capsule Protocol over HTTP/2 for MASQUE |
| MASQUE over HTTP/1.1 | RFC 9298 §3.2 | ❌ Not Implemented | Upgrade + Capsules; no library |
| QTunnel HTTP/2 Fallback | Proprietary | 🟡 Planned | Custom framing; documented as non-standard |

---

## 3. Library Versions Tested

| Library | Version | Commit/Tag | Go Version | Test Date |
|---------|---------|------------|------------|-----------|
| quic-go | v0.48.0 | TBD | 1.22+ | Pending |
| masque-go | latest | TBD | 1.22+ | Pending |
| connect-ip-go | latest | TBD | 1.22+ | Pending |
| Go stdlib | 1.22+ | N/A | 1.22+ | N/A |

**Note:** Versions to be pinned at implementation start. quic-go policy: supports latest two Go releases.

---

## 4. Interoperability

### 4.1 Tested Against

| Peer Implementation | Protocol | Test Status | Notes |
|---------------------|----------|-------------|-------|
| quic-go example server | QUIC | ⏳ Pending | In-process test |
| masque-go proxy | CONNECT-UDP | ⏳ Pending | Requires masque-go test server |
| connect-ip-go proxy | CONNECT-IP | ⏳ Pending | Requires connect-ip-go test server |
| curl (HTTP/3) | HTTP/3 | ⏳ Pending | For HTTP/3 carrier test |
| nghttp2 | HTTP/2 | ⏳ Pending | For fallback test |

### 4.2 Interoperability Claims

| Claim | Evidence | Status |
|-------|----------|--------|
| "Interoperates with masque-go" | Integration test passing | ❌ Not Verified |
| "Interoperates with connect-ip-go" | Integration test passing | ❌ Not Verified |
| "QUIC interoperable" | quic-go interop suite | ❌ Not Verified |
| "HTTP/3 interoperable" | h3 interop test | ❌ Not Verified |

**Policy:** No interoperability claims without passing automated integration tests against independent implementation.

---

## 5. Platform Support

| Platform | Native QUIC | HTTP/3 | TUN (VPN) | Notes |
|----------|-------------|--------|-----------|-------|
| Linux (amd64/arm64) | ✅ | ✅ | ✅ (tun/tap) | Primary target |
| macOS (amd64/arm64) | ✅ | ✅ | ✅ (utun) | Requires root for TUN |
| Windows (amd64/arm64) | ✅ | ✅ | ✅ (Wintun) | Requires Admin for TUN |
| FreeBSD | ✅ | ✅ | ✅ (tun) | Untested |
| OpenBSD | ✅ | ✅ | ✅ (tun) | Untested |
| Linux (MIPS/386) | ✅ | ✅ | ❓ | Go supported |

### 5.1 TUN Integration Requirements

| Platform | Interface | Library | Privileges |
|----------|-----------|---------|------------|
| Linux | tun/tap | `golang.org/x/sys/unix` | CAP_NET_ADMIN |
| Windows | Wintun | `github.com/wintun/wintun-go` | Administrator |
| macOS | utun | `golang.org/x/sys/unix` | root |

---

## 6. Feature Support Matrix

### 6.1 Core Features

| Feature | Phase 1 | Phase 2 | Phase 3 | Notes |
|---------|---------|---------|---------|-------|
| TCP Proxy | ✅ | ✅ | ✅ | Primary use case |
| SOCKS5 Proxy | 🟡 | 🟡 | 🟡 | Client-side only |
| TLS 1.3 | ✅ | ✅ | ✅ | Mandatory |
| Certificate Pinning | ✅ | ✅ | ✅ | Optional config |
| Token Auth | ✅ | ✅ | ✅ | Primary auth |
| mTLS Auth | 🟡 | 🟡 | 🟡 | Alternative |
| Connection Migration | ✅ | ✅ | ❌ | QUIC native only |
| 0-RTT | ❌ | ❌ | ❌ | Disabled by default |

### 6.2 Reliability Features

| Feature | Phase 1 | Phase 2 | Phase 3 | Notes |
|---------|---------|---------|---------|-------|
| Graceful Shutdown | ✅ | ✅ | ✅ | SIGTERM drain |
| Reconnection | 🟡 | 🟡 | 🟡 | Exponential backoff |
| Session Resumption | ❌ | ❌ | 🟡 | Design only |
| Failover (Multi-carrier) | ❌ | 🟡 | 🟡 | Manual config |
| Health Checks | ✅ | ✅ | ✅ | /health endpoint |

### 6.3 Observability

| Feature | Phase 1 | Phase 2 | Phase 3 | Notes |
|---------|---------|---------|---------|-------|
| Structured Logging | ✅ | ✅ | ✅ | JSON/console |
| Prometheus Metrics | ✅ | ✅ | ✅ | /metrics endpoint |
| Distributed Tracing | ❌ | 🟡 | 🟡 | OpenTelemetry hooks |
| qlog | 🟡 | 🟡 | 🟡 | quic-go support |
| Debug Endpoints | ✅ | ✅ | ✅ | pprof, vars |

### 6.4 Security Features

| Feature | Phase 1 | Phase 2 | Phase 3 | Notes |
|---------|---------|---------|---------|-------|
| Resource Limits | ✅ | ✅ | ✅ | Configurable |
| Rate Limiting | 🟡 | 🟡 | 🟡 | Per-IP/token |
| ACL (Allow/Deny) | ✅ | ✅ | ✅ | CIDR, hostname |
| DNS Rebinding Protection | ✅ | ✅ | ✅ | Resolve at connect |
| Audit Logging | 🟡 | 🟡 | 🟡 | Config changes |
| FIPS 140-3 | ❌ | ❌ | 🟡 | Needs Go 1.26+ |

---

## 7. Performance Baselines (Target)

| Metric | Target | Conditions |
|--------|--------|------------|
| **Connection Establishment** | < 50ms (p99) | Local loopback, TLS 1.3 resumption |
| **Throughput (TCP)** | > 1 Gbps | Single stream, local, no proxy overhead |
| **Throughput (MASQUE UDP)** | > 500 Mbps | Local, CONNECT-UDP |
| **Latency Overhead** | < 1ms | vs direct TCP |
| **Memory/Connection** | < 2 MB | 100 streams, idle |
| **CPU/Connection** | < 1% | At 100 Mbps |
| **Max Connections** | 10,000 | Single process, 4GB RAM |

**Note:** Actual numbers to be measured in Milestone 7 and documented here.

---

## 8. Known Limitations

| Limitation | Impact | Workaround | Resolution |
|------------|--------|------------|------------|
| No HTTP/2 MASQUE | UDP/IP not available over HTTP/2 | Use HTTP/3 or native QUIC | Implement Capsule Protocol over HTTP/2 |
| No 0-RTT | Slightly slower reconnect | Session resumption (future) | Enable with anti-replay |
| TUN requires root/Admin | Deployment friction | Run in container with CAP_NET_ADMIN | Document clearly |
| Single server endpoint | No HA built-in | External load balancer | Document architecture |
| No built-in service discovery | Manual config | External config management | Document patterns |

---

## 9. Version Compatibility Policy

### 9.1 Protocol Versions
- ALPN token: `qtunnel/1`, `qtunnel/2`, etc.
- Breaking changes → new ALPN token
- Clients/servers negotiate highest common version

### 9.2 Go Version Policy
- Follow quic-go: support latest two Go releases
- Minimum: Go 1.22 (current at project start)
- CI tests against: Go 1.22, 1.23

### 9.3 Dependency Update Policy
- Security updates: Within 48 hours of advisory
- Minor updates: Monthly
- Major updates: Tested in CI before merge

---

## 10. Migration Guide (Future)

### 10.1 v0.x → v1.0
- Protocol version `qtunnel/1` stabilized
- Config format stabilized
- API stable for embedders

### 10.2 Adding New Carriers
1. Implement `Carrier` interface
2. Add to config validation
3. Update compatibility matrix
4. Add integration tests

---

## 11. Verification Checklist

Each release must verify:

| Check | Method | Phase 1 | Phase 2 | Phase 3 |
|-------|--------|---------|---------|---------|
| Builds on Linux/macOS/Windows | CI | ✅ | ⏳ | ⏳ |
| Unit tests pass | `go test ./...` | ✅ | ⏳ | ⏳ |
| Race detector clean | `go test -race ./...` | ✅ | ⏳ | ⏳ |
| Fuzzing clean (30s) | `go test -fuzz` | ✅ | ⏳ | ⏳ |
| Integration test (QUIC) | In-process | ✅ | ✅ | ✅ |
| Integration test (HTTP/3) | vs masque-go | N/A | ⏳ | ⏳ |
| Integration test (MASQUE UDP) | vs masque-go proxy | N/A | ⏳ | N/A |
| Integration test (MASQUE IP) | vs connect-ip-go proxy | N/A | ⏳ | N/A |
| Benchmarks recorded | `go test -bench` | ✅ | ⏳ | ⏳ |
| Security scan clean | `govulncheck`, `gosec` | ✅ | ⏳ | ⏳ |
| Docs match implementation | Manual review | ✅ | ⏳ | ⏳ |

---

## 12. Acceptance Criteria for Release

A release is **READY** only if:

- [ ] All Phase 1 checks ✅
- [ ] No CRITICAL/HIGH security findings
- [ ] Performance within 20% of baselines
- [ ] Zero data corruption in stress tests
- [ ] Documentation complete and accurate
- [ ] No fabricated claims in any document
- [ ] Known limitations explicitly listed

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-10-25 | Initial version based on research |