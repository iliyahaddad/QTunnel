# QTunnel Research Report

**Research Date:** 2026-10-25  
**Researcher:** Kilo AI Assistant  
**Go Version Target:** Go 1.22+ (latest two releases per quic-go policy)

---

## 1. Upstream Sources Inspected

| Source | Version/Commit | Date Inspected | License |
|--------|----------------|----------------|---------|
| Go Documentation | Go 1.22+ | 2026-10-25 | BSD-3-Clause |
| quic-go | master (7,276 commits) | 2026-10-25 | MIT |
| masque-go | master (101 commits) | 2026-10-25 | MIT |
| connect-ip-go | master (97 commits) | 2026-10-25 | MIT |
| Xray-core/splithttp | main | 2026-10-25 | MPL-2.0 |
| paqet | master (159 commits) | 2026-10-25 | MIT |
| RFC 9000 (QUIC) | Published May 2021 | 2026-10-25 | IETF Trust |
| RFC 9001 (QUIC-TLS) | Published May 2021 | 2026-10-25 | IETF Trust |
| RFC 9002 (QUIC-CC) | Published May 2021 | 2026-10-25 | IETF Trust |
| RFC 9113 (HTTP/2) | Published June 2022 | 2026-10-25 | IETF Trust |
| RFC 9114 (HTTP/3) | Published June 2022 | 2026-10-25 | IETF Trust |
| RFC 9297 (HTTP Datagrams) | Published Aug 2022 | 2026-10-25 | IETF Trust |
| RFC 9298 (CONNECT-UDP) | Published Aug 2022 | 2026-10-25 | IETF Trust |
| RFC 9484 (CONNECT-IP) | Published 2023 | 2026-10-25 | IETF Trust |
| RFC 9220 (Extended CONNECT) | Published June 2022 | 2026-10-25 | IETF Trust |

---

## 2. Core Transport Libraries Analysis

### 2.1 quic-go

**Status:** Production-ready, actively maintained (11.8k stars, 202 watchers)

**Key Capabilities:**
- Full QUIC v1 (RFC 9000/9001/9002) implementation
- QUIC v2 (RFC 9369) support
- HTTP/3 (RFC 9114) with QPACK (RFC 9204)
- HTTP Datagrams (RFC 9297) and Capsule Protocol
- QUIC Datagrams (RFC 9221) for unreliable delivery
- Connection migration and multipath support
- qlog event logging
- Prometheus metrics
- FIPS 140-3 support with Go 1.26+
- Extensive fuzzing (OSS-Fuzz integrated)

**API Surface:**
- `quic.ListenAddr`, `quic.DialAddr` for raw QUIC connections
- `http3.Server`, `http3.RoundTripper` for HTTP/3
- Stream API: `OpenStreamSync`, `AcceptStream`, `Read`, `Write`, `Close`
- Datagram API: `SendDatagram`, `ReceiveDatagram`
- Configuration via `quic.Config` (timeouts, flow control, congestion control, keep-alive)

**Version Policy:** Supports latest two Go releases.

### 2.2 masque-go (CONNECT-UDP)

**Status:** Actively maintained implementation of RFC 9298

**Files:** client.go, proxy.go, conn.go, transport.go, request.go, proxy_request.go

**Capabilities:**
- Client and proxy (server) implementations
- HTTP/3 transport using quic-go HTTP/3
- CONNECT-UDP with Capsule Protocol and HTTP Datagrams
- Context ID support (0 for UDP payloads, extensible)
- URI Template configuration per RFC 9298 Section 2

**Limitations:**
- HTTP/3 only (no HTTP/2 fallback documented)
- Requires quic-go HTTP/3 stack

### 2.3 connect-ip-go (CONNECT-IP)

**Status:** Actively maintained implementation of RFC 9484

**Files:** client.go, proxy.go, conn.go, capsule.go, address.go, ip.go, ipv4.go, ipv6.go, dns.go, icmp.go, iprange.go, transport.go

**Capabilities:**
- Full CONNECT-IP per RFC 9484
- ADDRESS_ASSIGN, ADDRESS_REQUEST, ROUTE_ADVERTISEMENT capsules
- IPv4 and IPv6 support with prefix handling
- DNS resolution and PREF64 (draft-ietf-masque-connect-ip-dns-07)
- ICMP error signaling
- Remote Access VPN (Section 8.1) and Site-to-Site VPN (Section 8.2)
- TUN device integration examples in integration/
- Fuzzing support (conn_fuzz_test.go)

**Limitations:**
- HTTP/3 only
- Requires TUN device integration for full VPN (platform-specific)
- MTU considerations for IPv6 (minimum 1280 bytes per RFC 9484 Section 7.2.6)

### 2.4 Xray-core splithttp

**Status:** Reference implementation of custom HTTP-based transport

**Architecture:** Custom protocol over HTTP/1.1, HTTP/2, HTTP/3
- Bidirectional streaming via long-lived GET (download) + POST (upload)
- Custom framing with sequence numbers
- Mux implementation for stream multiplexing
- Padding for traffic obfuscation

**Key Difference:** Not standards-based MASQUE; proprietary protocol over HTTP.

### 2.5 paqet

**Status:** Raw packet tunneling with KCP+smux (not QUIC/HTTP-based)

**Architecture:** Raw TCP packets via pcap + KCP reliability + smux multiplexing
- Bypasses kernel TCP stack and firewall
- Requires root/Administrator and libpcap/Npcap
- Not applicable for standard QUIC/HTTP tunneling

---

## 3. MASQUE Standards Compliance Matrix

| Feature | RFC | quic-go | masque-go | connect-ip-go | Notes |
|---------|-----|---------|-----------|---------------|-------|
| HTTP Datagrams | 9297 | ✅ Full | ✅ Used | ✅ Used | Core transport |
| Capsule Protocol | 9297 | ✅ Full | ✅ Used | ✅ Used | Reliable control plane |
| QUIC Datagrams | 9221 | ✅ Full | ⚠️ Via HTTP/3 | ⚠️ Via HTTP/3 | Unreliable path |
| Extended CONNECT | 9220 | ✅ HTTP/3 | ✅ Used | ✅ Used | Required for HTTP/2,3 |
| CONNECT-UDP | 9298 | ❌ | ✅ Full | ❌ | masque-go only |
| CONNECT-IP | 9484 | ❌ | ❌ | ✅ Full | connect-ip-go only |
| HTTP/2 Fallback | 9298 §3.4 | ✅ HTTP/2 pkg | ❌ Not impl | ❌ Not impl | Capsule Protocol only |
| HTTP/1.1 Upgrade | 9298 §3.2 | ❌ | ❌ Not impl | ❌ Not impl | Upgrade + Capsules |

**Critical Finding:** Neither masque-go nor connect-ip-go implement HTTP/2 or HTTP/1.1 fallback. They are HTTP/3-only. Standards require capsule-based fallback for HTTP/2 but no Go library implements this yet.

---

## 4. Candidate Architecture Comparison

| Criterion | Native QUIC (A) | HTTP/3+MASQUE (B) | HTTP/2 Fallback (C) | WireGuard (D) |
|-----------|-----------------|-------------------|---------------------|---------------|
| **TCP Tunneling** | ✅ Native streams | ✅ Via HTTP/3 streams | ✅ Custom framing | ❌ Not designed for this |
| **UDP Tunneling** | ✅ QUIC Datagrams | ✅ CONNECT-UDP (RFC 9298) | ❌ No standard | ❌ No |
| **IP/VPN Tunneling** | ❌ Custom needed | ❌ Not in scope | ❌ No standard | ✅ Native |
| **Standards Compliance** | RFC 9000/9001/9002 | RFC 9297/9298/9114 | RFC 9113 + custom | RFC WireGuard |
| **Library Maturity** | ★★★★★ (quic-go) | ★★★★☆ (masque-go) | ★★★☆☆ (custom) | ★★★★★ (wireguard-go) |
| **Interoperability** | QUIC endpoints | MASQUE clients/servers | QTunnel-only | WireGuard peers |
| **Connection Migration** | ✅ Native | ✅ Via QUIC | ❌ TCP-based | ✅ Native |
| **Firewall Traversal** | UDP port 443 | HTTPS port 443 | HTTPS port 443 | UDP port 51820 |
| **Implementation Effort** | Medium | Medium-High | High (custom) | Low (use library) |
| **Security Review** | TLS 1.3 | TLS 1.3 + MASQUE | TLS 1.2+ + custom | Noise protocol |
| **Platform Support** | All Go platforms | All Go platforms | All Go platforms | All (needs TUN) |
| **Fallback Capability** | N/A | HTTP/2 via capsules* | Primary | N/A |

*HTTP/2 fallback requires implementing Capsule Protocol (RFC 9297) over HTTP/2 - no existing Go library does this for MASQUE.

---

## 5. Architecture Decision

### Selected: **Candidate A (Native QUIC) + Candidate B (HTTP/3 MASQUE) as Phase 2**

**Rationale:**

1. **Native QUIC (Phase 1 - MVP):**
   - Simplest working foundation for TCP tunneling
   - quic-go is the most mature, battle-tested library
   - Native streams = direct TCP proxy mapping (1 stream = 1 TCP connection)
   - No HTTP overhead for pure TCP tunneling
   - Connection migration, congestion control, flow control all built-in
   - Clear upgrade path to MASQUE

2. **HTTP/3 MASQUE (Phase 2):**
   - Standards-based UDP (CONNECT-UDP) and IP (CONNECT-IP) tunneling
   - masque-go and connect-ip-go provide working implementations
   - Interoperable with other MASQUE implementations
   - Same QUIC foundation, additive capability

3. **HTTP/2 Fallback (Phase 3 - Deferred):**
   - No existing library implements MASQUE over HTTP/2
   - Would require custom Capsule Protocol implementation
   - Can be added later if network environment demands it
   - Document as known limitation

4. **WireGuard (Not selected):**
   - Solves different problem (site-to-site VPN, not proxy/tunnel)
   - Could be integrated later for IP-layer VPN mode
   - Not a replacement for application-level proxying

---

## 6. Implementation Scope Decision

### Phase 1 (MVP - Milestones 1-3): **TCP Proxy over Native QUIC**
- Client ↔ Server over QUIC streams
- TLS 1.3 with certificate verification
- Authentication (token-based)
- Session management, resource limits, graceful shutdown
- Configuration via YAML
- End-to-end tests

### Phase 2 (Milestones 4-5): **MASQUE Integration**
- CONNECT-UDP for UDP proxying (using masque-go patterns)
- CONNECT-IP for IP-layer VPN (using connect-ip-go patterns)
- TUN device integration (platform-specific)

### Phase 3 (Future): **HTTP/2 Fallback**
- Custom implementation if needed
- Document clearly as QTunnel-specific protocol

---

## 7. Dependencies and Licensing

| Dependency | License | Compatible |
|------------|---------|------------|
| github.com/quic-go/quic-go | MIT | ✅ |
| github.com/quic-go/masque-go | MIT | ✅ |
| github.com/quic-go/connect-ip-go | MIT | ✅ |
| gopkg.in/yaml.v3 | MIT | ✅ |
| golang.org/x/net/http2 | BSD-3-Clause | ✅ |
| golang.org/x/crypto | BSD-3-Clause | ✅ |

**QTunnel License:** MIT (compatible with all dependencies)

---

## 8. Known Limitations and Risks

1. **No Go installed in environment** - Must install Go 1.22+ to build/test
2. **HTTP/2 MASQUE not implemented upstream** - Fallback requires custom work
3. **TUN integration requires platform-specific code** - Windows (Wintun), Linux (tun/tap), macOS (utun)
4. **Root/Administrator required for TUN** - VPN mode needs elevated privileges
5. **Firewall traversal not guaranteed** - QUIC/UDP may be blocked; HTTP/2/3 over TLS 443 more resilient
6. **No interoperability testing infrastructure** - Need independent MASQUE server/client for validation