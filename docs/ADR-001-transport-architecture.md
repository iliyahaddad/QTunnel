# ADR-001: Transport Architecture Decision

**Status:** Accepted  
**Date:** 2026-10-25  
**Deciders:** Kilo AI Assistant (Senior Go Networking Engineer)

---

## Context

QTunnel requires a modular transport architecture supporting multiple carrier protocols:
1. Native QUIC streams (TCP tunneling)
2. HTTP/3 with MASQUE (CONNECT-UDP, CONNECT-IP)
3. HTTP/2 fallback (when QUIC/UDP blocked)
4. Future: WireGuard integration for IP-layer VPN

The architecture must separate session/application logic from transport carriers.

---

## Decision

### Primary Architecture: Native QUIC (Phase 1) → HTTP/3 MASQUE (Phase 2)

**Phase 1 MVP: Native QUIC Transport**
- Use `github.com/quic-go/quic-go` directly for QUIC connections
- Native QUIC streams map 1:1 to TCP connections (proxy mode)
- TLS 1.3 with full certificate verification
- Custom application protocol over QUIC streams for session management

**Phase 2: HTTP/3 MASQUE Integration**
- Add `github.com/quic-go/masque-go` for CONNECT-UDP
- Add `github.com/quic-go/connect-ip-go` for CONNECT-IP
- HTTP/3 transport using quic-go HTTP/3 stack
- Shared session layer, carrier-specific implementations

**Phase 3 (Deferred): HTTP/2 Fallback**
- No existing Go library implements MASQUE over HTTP/2
- Would require custom Capsule Protocol (RFC 9297) over HTTP/2
- Implement only if network environment demands it
- Document as QTunnel-specific protocol, not standard MASQUE

---

## Carrier Interface Design

```go
// Minimal carrier interface - only what's needed
type Carrier interface {
    Name() string                    // "quic", "http3", "http2"
    Dial(ctx context.Context, cfg *Config) (Connection, error)
    Listen(cfg *Config) (Listener, error)
}

type Connection interface {
    OpenStream(ctx context.Context) (Stream, error)
    AcceptStream(ctx context.Context) (Stream, error)
    Close() error
    Context() context.Context
}

type Stream interface {
    io.Reader
    io.Writer
    io.Closer
    StreamID() int64
}

type Listener interface {
    Accept() (Connection, error)
    Close() error
    Addr() net.Addr
}
```

**Key Principle:** Do not force datagram carriers (QUIC Datagrams, HTTP Datagrams) into Stream interface. Model datagram capabilities explicitly when needed.

---

## Rejected Alternatives

### 1. Single HTTP/3-only Architecture
**Rejected because:** 
- No HTTP/2 fallback implemented in masque-go/connect-ip-go
- Overhead of HTTP/3 framing for simple TCP proxying
- QUIC native streams simpler for MVP

### 2. HTTP/2-first with MASQUE
**Rejected because:**
- No Go library implements CONNECT-UDP/CONNECT-IP over HTTP/2
- RFC 9298 requires Capsule Protocol over HTTP/2 - unimplemented
- Would block Phase 1 delivery

### 3. Custom protocol like Xray splithttp
**Rejected because:**
- Not standards-based
- No interoperability
- Security review burden higher
- Maintenance burden higher

### 4. WireGuard as primary transport
**Rejected because:**
- Solves different problem (IP-layer VPN, not application proxy)
- No TCP/UDP proxying semantics
- Could integrate later for VPN mode

---

## Trade-offs

| Aspect | Native QUIC | HTTP/3 MASQUE | HTTP/2 Fallback |
|--------|-------------|---------------|-----------------|
| **TCP Proxy** | ✅ Simple, direct | ✅ Via HTTP streams | ✅ Custom framing |
| **UDP Proxy** | ❌ Custom needed | ✅ RFC 9298 standard | ❌ No standard |
| **IP/VPN** | ❌ Custom needed | ✅ RFC 9484 standard | ❌ No standard |
| **Interop** | QUIC endpoints | MASQUE implementations | QTunnel-only |
| **Firewall** | UDP 443 may be blocked | HTTPS 443 usually open | HTTPS 443 usually open |
| **Complexity** | Low | Medium | High (custom) |

---

## Consequences

### Positive
- Phase 1 delivers working TCP tunnel quickly
- Standards-based UDP/IP in Phase 2 with interoperability
- Clear separation enables testing each carrier independently
- quic-go maturity reduces risk

### Negative
- Two transport stacks to maintain (QUIC native + HTTP/3)
- HTTP/2 fallback deferred (known gap)
- VPN mode requires TUN integration (platform complexity)

### Mitigations
- Shared session/authentication layer reduces duplication
- Carrier interface enables mock testing
- Document HTTP/2 gap explicitly in COMPATIBILITY.md
- TUN integration isolated in masque package

---

## Implementation Priority

1. **Milestone 2-3:** Native QUIC carrier + TCP proxy
2. **Milestone 4:** HTTP/3 carrier infrastructure
3. **Milestone 5:** CONNECT-UDP (masque-go integration)
4. **Milestone 5:** CONNECT-IP (connect-ip-go integration + TUN)
5. **Future:** HTTP/2 fallback if required

---

## Verification Criteria

- [ ] Native QUIC carrier passes integration tests
- [ ] Session layer works with both carriers
- [ ] MASQUE CONNECT-UDP interoperates with masque-go test server
- [ ] MASQUE CONNECT-IP interoperates with connect-ip-go test server
- [ ] HTTP/2 fallback documented as NOT IMPLEMENTED with clear gap analysis