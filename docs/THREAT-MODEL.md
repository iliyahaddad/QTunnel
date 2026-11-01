# QTunnel Threat Model

**Version:** 1.0  
**Date:** 2026-10-25  
**Classification:** Internal - Security Design Document

---

## 1. Assets and Trust Boundaries

### 1.1 Assets

| Asset | Description | Sensitivity |
|-------|-------------|-------------|
| User Traffic | Application data tunneled through QTunnel | HIGH |
| Authentication Credentials | Tokens, certificates, pre-shared keys | CRITICAL |
| Session State | Stream mappings, sequence numbers, flow control | HIGH |
| Configuration | Listen addresses, upstream proxies, ACLs | MEDIUM |
| Metadata | Connection timing, volume, endpoints | MEDIUM |
| Server Identity | TLS certificates, private keys | CRITICAL |

### 1.2 Trust Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│                     UNTRUSTED NETWORK                       │
│  (Internet, Public WiFi, Corporate Network, ISP)            │
└─────────────────────────┬───────────────────────────────────┘
                          │ TLS 1.3 / QUIC Crypto
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                      QTUNNEL CLIENT                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Local Apps  │──│  QTunnel    │──│  Network Interface  │  │
│  │ (Browser,   │  │  Client     │  │  (QUIC/UDP/TCP)     │  │
│  │  CLI, etc)  │  │  (Userland) │  │                     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────┬───────────────────────────────────┘
                          │ Authenticated Encrypted Tunnel
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                      QTUNNEL SERVER                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Network    │──│  QTunnel    │──│ Upstream/Destination│  │
│  │  Interface  │  │  Server     │  │  (Target Services)  │  │
│  │  (QUIC/UDP) │  │  (Userland) │  │                     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**Trust Assumptions:**
- Client trusts Server identity (certificate pinning or CA validation)
- Server trusts authenticated clients (token/mTLS)
- Local applications trust local QTunnel client (loopback)
- Upstream destinations trust QTunnel server (or not - transparent proxy)

---

## 2. Attacker Capabilities

### 2.1 Network Attacker (On-path)
- **Passive:** Observe timing, volume, packet sizes, endpoints
- **Active:** Drop, reorder, delay, replay, modify packets
- **Injection:** Inject packets (limited by TLS/QUIC integrity)
- **Blocking:** Block QUIC/UDP, force fallback to TCP/TLS

### 2.2 Endpoint Attacker
- **Compromised Client:** Malicious local application
- **Compromised Server:** Malicious server operator
- **Credential Theft:** Stolen tokens, certificates, keys

### 2.3 Protocol Attacker
- **Malformed Frames:** Invalid QUIC/HTTP/2/HTTP/3 frames
- **State Exhaustion:** Stream/connection/buffer flooding
- **Logic Flaws:** Race conditions, use-after-free, integer overflow

---

## 3. Security Controls

### 3.1 Authentication and Authorization

| Control | Implementation | Threat Mitigated |
|---------|----------------|------------------|
| **Server Authentication** | TLS 1.3 with certificate verification (system CA + optional pinning) | MITM, Impersonation |
| **Client Authentication** | Bearer token (configurable) or mTLS | Unauthorized access |
| **Session Binding** | Tokens bound to TLS session / QUIC connection | Session hijacking |
| **Destination Authorization** | Server-side ACL (allowlist/denylist) | Abuse, pivot attacks |

**Token Requirements:**
- Minimum 256-bit entropy (32 bytes cryptographically random)
- Constant-time comparison
- Rotation support without downtime
- Revocation via config reload

### 3.2 Transport Security

| Layer | Protocol | Security Properties |
|-------|----------|---------------------|
| **QUIC Native** | QUIC v1 + TLS 1.3 | Confidentiality, Integrity, Authenticity, Forward Secrecy |
| **HTTP/3 MASQUE** | HTTP/3 + QUIC + TLS 1.3 | Same + HTTP semantics |
| **HTTP/2 Fallback** | HTTP/2 + TLS 1.2+ | Confidentiality, Integrity (no 0-RTT) |

**Mandatory:**
- TLS 1.3 minimum for QUIC/HTTP/3
- TLS 1.2 minimum for HTTP/2 (TLS 1.3 preferred)
- Certificate verification **never disabled** in production config
- ALPN validation: `h3` for HTTP/3, `h2` for HTTP/2

### 3.3 Replay and Session Hijacking Protection

| Mechanism | Scope |
|-----------|-------|
| **QUIC Connection IDs** | Unique per connection, rotation supported |
| **TLS Session Tickets** | Encrypted, forward-secure, single-use preferred |
| **Application Session Tokens** | Opaque, high-entropy, bound to transport |
| **Stream IDs** | QUIC-managed, monotonic, non-reusable |
| **0-RTT Replay Protection** | Disabled by default; opt-in with anti-replay window |

### 3.4 Resource Exhaustion Mitigations

| Resource | Limit | Enforcement |
|----------|-------|-------------|
| **Concurrent Connections** | Configurable (default: 1000) | Listener backlog + semaphore |
| **Concurrent Streams/Connection** | QUIC: 100 (configurable) | QUIC config `MaxIncomingStreams` |
| **Stream Receive Buffer** | 1 MB per stream (configurable) | Flow control window |
| **Connection Receive Buffer** | 10 MB (configurable) | QUIC config `MaxReceiveBufferSize` |
| **Datagram Queue** | 64 KB (configurable) | Drop oldest on overflow |
| **Handshake Timeout** | 10s (configurable) | `quic.Config.HandshakeIdleTimeout` |
| **Idle Timeout** | 30s (configurable) | `quic.Config.MaxIdleTimeout` |
| **Max Frame Size** | 16 KB (configurable) | Frame parser validation |

### 3.5 Input Validation

- **Frame Parsing:** Bounds-checked, no allocation before length validation
- **Variable-Length Integers:** Maximum 8 bytes (RFC 9000 Section 16)
- **URI Templates:** Validated per RFC 6570 Level 3
- **Target Host/Port:** DNS name validation, port range 1-65535
- **Capsule Types:** Unknown types silently ignored (RFC 9297)
- **Context IDs:** 62-bit, validated per RFC 9297/9298/9484

### 3.6 Cryptographic Hygiene

| Requirement | Implementation |
|-------------|----------------|
| **Randomness** | `crypto/rand` only for secrets, tokens, nonces |
| **Constant-Time Compare** | `subtle.ConstantTimeCompare` for tokens/secrets |
| **Key Derivation** | TLS 1.3 exporters / HKDF for application keys |
| **No Hardcoded Secrets** | All secrets from config/environment |
| **Certificate Validation** | Full chain, hostname, expiry, revocation (OCSP/CRL) |
| **Algorithm Agility** | TLS 1.3 cipher suites only (AES-GCM, ChaCha20-Poly1305) |

---

## 4. Specific Threat Analysis

### 4.1 TLS/QUIC Handshake Attacks

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| **Downgrade to TLS 1.2** | Low | Medium | Enforce MinVersion=TLS1.3 in QUIC config |
| **Certificate Spoofing** | Low | Critical | Pinning option; CA validation mandatory |
| **0-RTT Replay** | Medium | Medium | Disable 0-RTT by default; application-level anti-replay |
| **Key Compromise** | Low | Critical | Short-lived certs; key rotation automation |

### 4.2 Protocol Parsing Attacks

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| **QUIC Frame Injection** | Medium | High | quic-go validated; fuzzing integrated |
| **HTTP/3 Header Smuggling** | Low | High | QPACK decoder limits; no intermediary trust |
| **Capsule Protocol Confusion** | Medium | Medium | Strict type/length validation; unknown=drop |
| **Datagram Reordering/Replay** | Medium | Low | Context IDs; application handles reliability |

### 4.3 Resource Exhaustion (DoS)

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| **Connection Flood** | High | High | Rate limiting; connection limits; SYN cookies (OS) |
| **Stream Flood** | High | High | Stream limits; flow control; idle timeout |
| **Buffer Exhaustion** | Medium | High | Bounded queues; backpressure; drop policies |
| **CPU Exhaustion (Crypto)** | Medium | Medium | QUIC offloads to Go TLS; limit handshakes/sec |

### 4.4 Application-Layer Threats

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| **Open Proxy Abuse** | High | Critical | Destination ACL; authentication required |
| **DNS Rebinding** | Medium | High | Resolve at connection time; pin IPs; TTL respect |
| **Internal Service Access** | High | Critical | Denylist RFC1918, localhost, link-local by default |
| **Traffic Analysis** | High | Medium | Padding (optional); constant-rate (future) |

### 4.5 MASQUE-Specific Threats

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| **CONNECT-UDP Amplification** | Medium | High | Validate target; rate limit; source IP validation |
| **CONNECT-IP Route Injection** | Medium | Critical | Route advertisement validation; prefix length checks |
| **Capsule Malformation** | Medium | High | Strict parsing; fuzzing; length bounds |
| **MTU Violation (IPv6)** | Low | Medium | Enforce 1280 minimum; ICMPv6 PTB handling |

---

## 5. Logging and Metadata Exposure

### 5.1 What MUST NOT Be Logged
- Authentication tokens/secrets
- Payload data (application traffic)
- TLS session keys
- Certificate private keys

### 5.2 What MAY Be Logged (with care)
- Connection metadata (time, duration, bytes, remote IP)
- Stream counts, errors
- Authentication success/failure (not token values)
- Configuration changes (audit trail)

### 5.3 Log Sanitization
- Structured logging with field-level redaction
- Debug logs separate from audit logs
- No PII in logs unless explicitly configured

---

## 6. Residual Risks

| Risk | Description | Acceptance |
|------|-------------|------------|
| **Traffic Classification** | QUIC/HTTP/3 fingerprints identifiable | Accepted - encryption ≠ invisibility |
| **Timing/Volume Analysis** | Correlation attacks possible | Accepted - out of scope for v1 |
| **Endpoint Compromise** | Stolen tokens = full access | Mitigated by rotation, short TTL |
| **Implementation Bugs** | Memory safety, logic errors | Mitigated by Go, fuzzing, review |
| **Dependency Vulnerabilities** | quic-go, stdlib CVEs | Mitigated by Dependabot, Go updates |
| **Configuration Errors** | Misconfigured ACLs, weak tokens | Mitigated by validation, examples |

---

## 7. Security Testing Requirements

### 7.1 Mandatory Tests
- [ ] TLS certificate validation (expired, wrong host, self-signed)
- [ ] Token authentication (valid, invalid, expired, replay)
- [ ] Resource limits (connection, stream, buffer exhaustion)
- [ ] Graceful shutdown under load
- [ ] Malformed frame handling (fuzzing)
- [ ] Concurrent connection/stream stress

### 7.2 MASQUE-Specific Tests (Phase 2)
- [ ] CONNECT-UDP target validation (denylist, RFC1918)
- [ ] CONNECT-IP route advertisement validation
- [ ] Capsule parsing fuzzing
- [ ] MTU edge cases (IPv6 1280 minimum)
- [ ] ICMP error signaling

### 7.3 Penetration Testing
- External audit before v1.0 release
- Focus: authentication bypass, resource exhaustion, logic flaws

---

## 8. Incident Response

### 8.1 Compromise Indicators
- Unexpected connection spikes
- Authentication failure bursts
- Unknown destination accesses
- Certificate validation failures

### 8.2 Response Procedures
1. **Token Revocation:** Config reload with new token list
2. **Certificate Rotation:** Deploy new cert, reload server
3. **Rate Limiting:** Emergency config update
4. **Full Shutdown:** SIGTERM grace period (30s default)

---

## 9. Compliance Notes

- **No FIPS 140-3** in v1 (requires Go 1.26+ and quic-go v0.60+)
- **No HIPAA/GDPR** specific controls (application responsibility)
- **Export Controls:** Standard TLS/QUIC cryptography (unrestricted)

---

## 10. Future Enhancements

- [ ] Post-quantum KEM (Kyber) when standardized in TLS
- [ ] Traffic shaping / padding for timing analysis resistance
- [ ] Hardware-backed key storage (TPM, HSM)
- [ ] Formal verification of protocol state machines
- [ ] Zero-trust network access (ZTNA) integration
