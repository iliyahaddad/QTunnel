# QTunnel Security Policy

**Version:** 1.0  
**Date:** 2026-10-25

---

## 1. Reporting Security Vulnerabilities

**DO NOT** report security vulnerabilities via public GitHub issues.

### 1.1 Reporting Channel

For any questions, feedback, or suggestions, please leave a comment or open a new issue on GitHub.

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### 1.2 Response Timeline

| Severity | Acknowledgment | Fix Target |
|----------|----------------|------------|
| Critical | 24 hours | 7 days |
| High | 48 hours | 14 days |
| Medium | 7 days | 30 days |
| Low | 14 days | 90 days |

### 1.3 Disclosure Policy

- Coordinated disclosure preferred
- 90-day disclosure deadline from report
- CVE requested for qualifying issues
- Credit given to reporters (unless anonymous requested)

---

## 2. Supported Versions

| Version | Supported | End of Life |
|---------|-----------|-------------|
| 0.x (pre-1.0) | Best effort | N/A |
| 1.x (current) | ✅ Active | TBD |
| 1.x-1 (previous) | Security only | 6 months after 1.x+1 |

---

## 3. Security Architecture Summary

### 3.1 Transport Security
- **QUIC Native:** TLS 1.3 (RFC 9001) via quic-go
- **HTTP/3:** TLS 1.3 + HTTP/3 (RFC 9114) via quic-go
- **Certificate Validation:** Mandatory, system CA + optional pinning
- **Cipher Suites:** TLS 1.3 only (AES-GCM, ChaCha20-Poly1305)

### 3.2 Authentication
- **Primary:** Pre-shared tokens (256-bit entropy, constant-time compare)
- **Alternative:** mTLS (client certificates)
- **Token Rotation:** Hot reload via config file watch
- **Session Binding:** Token validated per connection

### 3.3 Authorization
- **Destination ACL:** Allowlist/denylist (CIDR, hostnames)
- **Default Denylist:** RFC1918, localhost, link-local, multicast
- **DNS Resolution:** At connection time, TTL respected

### 3.4 Resource Protection
- Connection limits (default: 1000)
- Stream limits (default: 100/connection)
- Buffer limits (stream: 1MB, connection: 10MB)
- Timeouts: handshake (10s), idle (30s)
- Rate limiting (planned)

---

## 4. Secure Configuration Guidelines

### 4.1 Server Configuration

```yaml
# SECURE DEFAULTS - DO NOT WEAKEN
server:
  listen:
    addr: ":4433"          # Non-standard port recommended
tls:
  cert_file: "server.crt"  # Valid, non-expired cert
  key_file: "server.key"   # Protected (chmod 600)
auth:
  tokens: ["..."]          # 32-byte base64, rotate periodically
  # OR token_file: "tokens.txt"  # One per line, watched for changes
proxy:
  allowlist: ["specific.dest:443"]  # Restrict destinations
  denylist: 
    - "10.0.0.0/8"       # RFC1918
    - "127.0.0.0/8"      # Localhost
    - "::1"              # IPv6 localhost
    - "169.254.0.0/16"   # Link-local
    - "224.0.0.0/4"      # Multicast
limits:
  max_connections: 1000
  max_streams_per_conn: 100
```

### 4.2 Client Configuration

```yaml
client:
  server:
    addr: "server.example.com:4433"
    sni: "server.example.com"
    insecure_skip_verify: false  # NEVER true in production
  auth:
    token: "..."  # Same 32-byte token
```

### 4.3 Certificate Management

- Use short-lived certificates (Let's Encrypt: 90 days)
- Automate renewal (cert-manager, acme.sh, etc.)
- Monitor expiry (alert at 30 days)
- Private keys: file permissions 600, owned by qtunnel user

---

## 5. Threat Model Highlights

See `docs/THREAT-MODEL.md` for full model. Key points:

| Threat | Mitigation |
|--------|------------|
| MITM | TLS 1.3 + certificate pinning option |
| Unauthorized access | Token auth + ACL |
| DoS (resource exhaustion) | Connection/stream/buffer limits |
| Open proxy abuse | Default denylist + allowlist |
| Credential theft | Token rotation, short TTL |
| Traffic analysis | Encryption (QUIC/TLS); padding planned |

---

## 6. Cryptographic Implementation

### 6.1 Libraries
- **TLS:** Go standard library (`crypto/tls`) - FIPS 140-3 validated modules available
- **QUIC Crypto:** quic-go (uses Go TLS)
- **Randomness:** `crypto/rand` only
- **Comparison:** `crypto/subtle.ConstantTimeCompare`

### 6.2 Algorithms
| Purpose | Algorithm |
|---------|-----------|
| Transport Encryption | TLS 1.3 (AES-256-GCM, ChaCha20-Poly1305) |
| Key Derivation | TLS 1.3 HKDF |
| Token Generation | `crypto/rand` 32 bytes |
| Token Comparison | Constant-time |

### 6.3 What We DON'T Do
- ❌ Custom cryptography
- ❌ Hardcoded keys/secrets
- ❌ Disabled certificate verification (in production config)
- ❌ Legacy TLS (< 1.2)
- ❌ Weak cipher suites

---

## 7. Dependency Security

### 7.1 Policy
- Pin dependency versions in `go.mod`
- `govulncheck` in CI on every PR
- Dependabot alerts enabled
- Weekly dependency review

### 7.2 Critical Dependencies
| Dependency | Risk | Monitoring |
|------------|------|------------|
| quic-go | Transport security | quic-go security advisories |
| Go stdlib | TLS, crypto | Go security releases |
| masque-go | MASQUE implementation | Upstream security |
| connect-ip-go | VPN implementation | Upstream security |

---

## 8. Secure Development Practices

### 8.1 Code Review
- All changes require review
- Security-focused review for: auth, parsing, crypto, network
- `gosec` static analysis in CI

### 8.2 Testing
- Fuzzing on all parsers (frame, config, capsule)
- Race detector on all tests
- Integration tests with malicious inputs

### 8.3 Release Process
- Signed releases (cosign/sigstore planned)
- Reproducible builds
- SBOM generation (planned)

---

## 9. Incident Response

### 9.1 Compromise Indicators
- Unusual connection patterns
- Authentication failure spikes
- Unknown destinations in logs
- Certificate validation failures

### 9.2 Response Actions
1. **Immediate:** Rotate tokens, reload config
2. **Short-term:** Block suspicious IPs, increase logging
3. **Long-term:** Certificate rotation, audit access

---

## 10. Compliance

- **FIPS 140-3:** Not supported in v1 (requires Go 1.26+)
- **Common Criteria:** Not evaluated
- **Export Controls:** Standard TLS/QUIC (unrestricted)

---

## 11. Contact

For any questions, feedback, or suggestions, please leave a comment or open a new issue on GitHub.
PGP Key: (to be published)

---

## 12. History

| Date | Version | Changes |
|------|---------|---------|
| 2026-10-25 | 1.0 | Initial policy |
