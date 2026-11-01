# QTunnel Protocol Specification

**Version:** 1.0  
**Date:** 2026-10-25  
**Status:** Draft - Phase 1 (Native QUIC)

---

## 1. Overview

QTunnel uses a layered protocol architecture:

```
┌─────────────────────────────────────┐
│        Application Layer            │
│  (TCP Proxy / SOCKS5 / VPN)         │
├─────────────────────────────────────┤
│        Session Layer                │
│  (Auth, Stream Mux, Flow Control)   │
├─────────────────────────────────────┤
│        Frame Layer                  │
│  (Typed Frames over Streams)        │
├─────────────────────────────────────┤
│        Carrier Layer                │
│  QUIC Native / HTTP/3 / HTTP/2      │
└─────────────────────────────────────┘
```

This document specifies the **Frame Layer** (application protocol over carrier streams) and **Session Layer** semantics.

---

## 2. Carrier Layer Requirements

### 2.1 Native QUIC (Phase 1)
- **Protocol:** QUIC v1 (RFC 9000) + TLS 1.3 (RFC 9001)
- **ALPN:** `qtunnel/1` (custom, not standard)
- **Streams:** Bidirectional, reliable, ordered byte streams
- **Stream Mapping:** 1 QUIC stream = 1 proxied TCP connection
- **Connection ID:** Used for migration; rotation supported

### 2.2 HTTP/3 (Phase 2)
- **Protocol:** HTTP/3 (RFC 9114) over QUIC
- **ALPN:** `h3`
- **Extended CONNECT:** RFC 9220 for stream tunneling
- **Settings:** `ENABLE_CONNECT_PROTOCOL` (0x08) = 1
- **Stream Mapping:** 1 HTTP request stream = 1 proxied connection

### 2.3 HTTP/2 Fallback (Phase 3 - Deferred)
- **Protocol:** HTTP/2 (RFC 9113) over TLS 1.2+
- **ALPN:** `h2`
- **Custom Framing:** QTunnel-specific (not MASQUE)
- **Documented in COMPATIBILITY.md as non-standard**

---

## 3. Frame Layer Specification

### 3.1 Frame Format

All frames use variable-length integer encoding (RFC 9000 Section 16):

```
Frame {
  Type (varint),
  Flags (varint),
  Length (varint),
  Payload (Length bytes),
}
```

**Maximum Frame Size:** 16 KB (configurable, default)
**Maximum Varint:** 8 bytes (2^62 - 1)

### 3.2 Frame Types

| Type | Value | Name | Direction | Description |
|------|-------|------|-----------|-------------|
| 0x00 | 0 | RESERVED | - | Reserved for extension |
| 0x01 | 1 | CONNECT | C→S | Request new proxied connection |
| 0x02 | 2 | CONNECT_OK | S→C | Connection established |
| 0x03 | 3 | CONNECT_FAIL | S→C | Connection failed |
| 0x04 | 4 | DATA | Both | Application data |
| 0x05 | 5 | CLOSE | Both | Close stream gracefully |
| 0x06 | 6 | PING | Both | Keep-alive / RTT measurement |
| 0x07 | 7 | PONG | Both | Response to PING |
| 0x08 | 8 | AUTH | C→S | Authentication (if not in handshake) |
| 0x09 | 9 | AUTH_OK | S→C | Authentication successful |
| 0x0A | 10 | AUTH_FAIL | S→C | Authentication failed |
| 0x0B | 11 | WINDOW_UPDATE | Both | Flow control credit |
| 0x0C | 12 | GOAWAY | Both | Graceful shutdown |

### 3.3 Frame Flags

| Flag | Value | Meaning |
|------|-------|---------|
| FIN | 0x01 | Last frame on this stream (for DATA) |
| ACK | 0x02 | Requires acknowledgment (for PING) |
| PRIORITY | 0x04 | High priority (for CONNECT) |

### 3.4 Frame Payloads

#### CONNECT (0x01)
```
CONNECT Payload {
  Destination Address (varint length + bytes),
  Destination Port (varint),
  Protocol (varint),  // 0=TCP, 1=UDP (future)
}
```
- Address: UTF-8 hostname or IP literal
- Port: 1-65535
- Protocol: Reserved for future UDP support

#### CONNECT_OK (0x02)
```
CONNECT_OK Payload {
  Stream ID (varint),  // Echo of initiating stream
  Assigned Address (optional, varint length + bytes),
}
```

#### CONNECT_FAIL (0x03)
```
CONNECT_FAIL Payload {
  Error Code (varint),
  Error Message (varint length + UTF-8 bytes),
}
```

**Error Codes:**
| Code | Name | Description |
|------|------|-------------|
| 1 | INVALID_DESTINATION | Malformed address/port |
| 2 | DENIED | ACL rejection |
| 3 | DNS_FAILED | Resolution failed |
| 4 | CONNECTION_REFUSED | Target refused |
| 5 | TIMEOUT | Connection timeout |
| 6 | RESOURCE_EXHAUSTED | Limits reached |
| 7 | INTERNAL_ERROR | Server error |

#### DATA (0x04)
```
DATA Payload {
  Data (Length bytes),  // Raw application data
}
```
- FIN flag indicates end of stream
- Flow controlled by WINDOW_UPDATE

#### CLOSE (0x05)
```
CLOSE Payload {
  Error Code (varint),  // 0 = normal, >0 = error
}
```

#### PING (0x06) / PONG (0x07)
```
PING Payload {
  Timestamp (varint, microseconds since epoch),
  Echo Token (8 bytes, opaque),
}
```
- PONG echoes Timestamp and Echo Token
- Used for RTT measurement and keep-alive

#### AUTH (0x08)
```
AUTH Payload {
  Token (varint length + bytes),  // Base64 or raw
}
```
- Sent immediately after connection if not using TLS client cert
- Server responds with AUTH_OK or AUTH_FAIL

#### WINDOW_UPDATE (0x0B)
```
WINDOW_UPDATE Payload {
  Credit (varint),  // Bytes added to receive window
}
```
- Initial window: 64 KB per stream
- Connection-level window: 1 MB

#### GOAWAY (0x0C)
```
GOAWAY Payload {
  Error Code (varint),
  Last Stream ID (varint),
  Debug Message (varint length + UTF-8 bytes),
}
```
- Initiates graceful shutdown
- No new streams after GOAWAY sent

---

## 4. Session Layer

### 4.1 Connection Lifecycle

```
Client                                    Server
  │                                       │
  ├─ QUIC Handshake (TLS 1.3) ───────────►│
  │◄──────────────────────────────────────┤
  │                                       │
  ├─ AUTH (if token-based) ──────────────►│
  │◄──────── AUTH_OK ─────────────────────┤
  │                                       │
  ├─ CONNECT (stream 1) ─────────────────►│
  │◄──────── CONNECT_OK ──────────────────┤
  │                                       │
  ├─ DATA ... (bidirectional) ───────────►│
  │                                       │
  ├─ CLOSE ──────────────────────────────►│
  │◄──────── CLOSE ───────────────────────┤
  │                                       │
  ├─ GOAWAY ─────────────────────────────►│
  │◄──────── GOAWAY ──────────────────────┤
  │                                       │
  └─ QUIC Close ─────────────────────────►│
```

### 4.2 Stream Lifecycle

Each proxied TCP connection maps to one QUIC/HTTP stream:

```
Stream States:
IDLE → OPEN → HALF_CLOSED_LOCAL → CLOSED
           ↓
      HALF_CLOSED_REMOTE → CLOSED
```

- **OPEN:** CONNECT sent, CONNECT_OK received
- **HALF_CLOSED_LOCAL:** Local sent CLOSE/FIN, remote not yet
- **HALF_CLOSED_REMOTE:** Remote sent CLOSE/FIN, local not yet
- **CLOSED:** Both sides closed

### 4.3 Flow Control

**Stream-Level:**
- Initial receive window: 64 KB
- WINDOW_UPDATE adds credit
- Sender tracks remote window; blocks on zero

**Connection-Level:**
- Initial receive window: 1 MB
- Aggregate of all stream windows
- Prevents head-of-line blocking across streams

### 4.4 Resource Limits

| Resource | Default | Configurable |
|----------|---------|--------------|
| Max concurrent connections | 1000 | Yes |
| Max streams per connection | 100 | Yes |
| Stream receive buffer | 1 MB | Yes |
| Connection receive buffer | 10 MB | Yes |
| Handshake timeout | 10s | Yes |
| Idle timeout | 30s | Yes |
| Max frame size | 16 KB | Yes |

---

## 5. Authentication Protocol

### 5.1 Token-Based (Primary)

1. Client generates/receives 256-bit token (base64-encoded)
2. Token configured on server (list or file)
3. On connection: Client sends AUTH frame with token
4. Server: Constant-time compare, responds AUTH_OK/AUTH_FAIL
5. Token rotation: Server reloads token file on SIGHUP or config watch

### 5.2 mTLS (Alternative)

- Client presents certificate
- Server validates against CA / known certificates
- No AUTH frame needed; identity from TLS

### 5.3 Session Binding

- Token validated per connection
- QUIC connection ID rotation doesn't affect auth
- Session resumption (future): token + ticket

---

## 6. Proxy Protocol Semantics

### 6.1 TCP Proxy Mode

1. Local application connects to QTunnel client listener
2. Client opens QUIC stream, sends CONNECT with target host:port
3. Server validates ACL, dials target
4. Server responds CONNECT_OK or CONNECT_FAIL
5. Bidirectional DATA frames ↔ TCP socket
6. Either side sends CLOSE on EOF/error

### 6.2 SOCKS5 Mode (Future)

- Client implements SOCKS5 server locally
- CONNECT frame includes SOCKS5 target address
- Server performs SOCKS5 handshake with target (if needed)

### 6.3 Destination Authorization

**Server-Side ACL:**
- Allowlist: CIDR ranges, hostnames, "*" for all
- Denylist: RFC1918, localhost, link-local by default
- DNS resolution at connection time (not config time)
- Result cached with TTL respect

---

## 7. Error Handling

### 7.1 Frame-Level Errors
- Unknown frame type: Ignore (forward compatibility)
- Malformed varint: Close stream with PROTOCOL_ERROR
- Length exceeds max: Close connection with FRAME_TOO_LARGE
- Unexpected frame in state: Close stream with STREAM_STATE_ERROR

### 7.2 Connection-Level Errors
- Authentication failure: Close connection with AUTH_FAILED
- Resource limit exceeded: CONNECT_FAIL with RESOURCE_EXHAUSTED
- Idle timeout: GOAWAY then close
- TLS alert: Propagate as connection close

### 7.3 Application Errors
- Target connection refused: CONNECT_FAIL with CONNECTION_REFUSED
- Target timeout: CONNECT_FAIL with TIMEOUT
- Target reset: CLOSE with error code

---

## 8. Keep-Alive and Health

### 8.1 PING/PONG
- Interval: 10s (configurable)
- Timeout: 3 missed = connection dead
- Measures RTT for adaptive timeouts

### 8.2 QUIC Keep-Alive
- QUIC PING frames (distinct from application PING)
- Configured via `quic.Config.KeepAlivePeriod`

---

## 9. Version Negotiation

### 9.1 Protocol Version
- Carried in ALPN: `qtunnel/1`, `qtunnel/2`, etc.
- No in-band version negotiation (ALPN decides)

### 9.2 Frame Version
- Frame format version fixed per protocol version
- New frame types: ignored if unknown (extensibility)

---

## 10. Security Considerations

### 10.1 Frame Parsing
- Always validate Length before allocation
- Maximum frame size enforced
- Varint decoding bounds-checked

### 10.2 Replay Protection
- QUIC/TLS provides transport replay protection
- Application PING uses timestamp + echo token
- 0-RTT disabled by default

### 10.3 Traffic Analysis Resistance
- Padding: Optional random padding on DATA frames (future)
- Constant-rate: Not implemented (future)

---

## 11. IANA Considerations

- ALPN token `qtunnel/1` - Private use (no IANA registration)
- Frame types - Private registry (this spec)
- Error codes - Private registry (this spec)

---

## 12. Changes from Standards

| Standard | Deviation | Reason |
|----------|-----------|--------|
| MASQUE | Not used in Phase 1 | Simpler TCP proxy |
| HTTP/3 Extended CONNECT | Not used in Phase 1 | Native QUIC simpler |
| TLS Client Cert | Optional | Token easier for ops |
| QUIC Datagrams | Not used | TCP proxy doesn't need |

---

## 13. Future Extensions

### 13.1 Session Resumption
- Ticket-based (TLS session ticket + application token)
- Stream state reconstruction
- Sequence numbers for in-flight data

### 13.2 Multipath
- QUIC multipath extension (draft-ietf-quic-multipath)
- Multiple paths per connection

### 13.3 Datagram Support
- QUIC Datagrams for UDP proxy (Phase 2 MASQUE)
- Unreliable delivery option

---

## Appendix A: Frame Encoding Examples

### CONNECT Request
```
Type=1(1) Flags=0(1) Length=18(1) Payload=[
  AddrLen=9(1) "example.com"(9)
  Port=443(2)
  Protocol=0(1)
]
Wire: 01 00 12 09 65 78 61 6d 70 6c 65 2e 63 6f 6d 02 bb 01 00
```

### DATA Frame (100 bytes, FIN)
```
Type=4(1) Flags=1(1) Length=100(1) Payload=[100 bytes]
Wire: 04 01 64 [100 bytes]
```

### WINDOW_UPDATE (64 KB credit)
```
Type=11(1) Flags=0(1) Length=2(1) Payload=[0x10 0x00]  // 65536
Wire: 0b 00 02 10 00
```

---

## Appendix B: Varint Encoding (RFC 9000 Section 16)

| Value Range | Prefix | Length |
|-------------|--------|--------|
| 0 - 63 | 00 | 1 byte |
| 64 - 16383 | 01 | 2 bytes |
| 16384 - 1073741823 | 10 | 4 bytes |
| 1073741824 - 4611686018427387903 | 11 | 8 bytes |

Example: 443 = 0x01BB → `40 01 BB` (2-byte encoding)
