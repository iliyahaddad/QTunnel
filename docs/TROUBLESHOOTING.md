# QTunnel Troubleshooting Guide

**Version:** 1.0  
**Date:** 2026-10-25

---

## 1. Quick Diagnostic Commands

### 1.1 Check Binary Version
```bash
qtunnel-client version
qtunnel-server version
```

### 1.2 Validate Configuration
```bash
qtunnel-client check-config -c client.yaml
qtunnel-server check-config -c server.yaml
```

### 1.3 Test Connectivity
```bash
qtunnel-diagnose ping -c client.yaml
qtunnel-diagnose cert -c client.yaml
```

---

## 2. Common Issues

### 2.1 Connection Failures

#### "connection refused"
```
Error: dial tcp server.example.com:4433: connect: connection refused
```
**Causes:**
- Server not running
- Wrong port in config
- Firewall blocking port

**Fix:**
```bash
# Check server is listening
ss -ltnp | grep 4433
# Or on Windows
netstat -an | findstr 4433

# Verify config matches
grep -A2 "listen:" server.yaml
```

#### "timeout"
```
Error: context deadline exceeded
```
**Causes:**
- Network latency > handshake timeout (default 10s)
- UDP blocked (QUIC requires UDP)
- Server overloaded

**Fix:**
```yaml
# Increase timeouts in client config
transport:
  quic:
    handshake_timeout: 30s
    max_idle_timeout: 60s
```

#### "certificate verification failed"
```
Error: tls: failed to verify certificate: x509: certificate signed by unknown authority
```
**Causes:**
- Self-signed cert without CA trust
- Wrong SNI
- Cert expired

**Fix:**
```yaml
# Option 1: Add CA to system trust
# Option 2: Use cert pinning (provide server cert hash)
# Option 3: For dev only - DANGEROUS
insecure_skip_verify: true  # NEVER in production
```

#### "authentication failed"
```
Error: auth failed: invalid token
```
**Causes:**
- Token mismatch client/server
- Token file not reloaded
- Token format wrong (base64 vs raw)

**Fix:**
```bash
# Verify token matches
cat client.yaml | grep token
cat server.yaml | grep -A5 tokens

# Generate new token
qtunnel-diagnose secret
```

---

### 2.2 Performance Issues

#### Low Throughput
**Symptoms:** Transfer speed << expected

**Diagnosis:**
```bash
# Check CPU
top -p $(pgrep qtunnel)

# Check memory
pmap $(pgrep qtunnel)

# Check QUIC stats (if exposed)
curl localhost:6060/debug/vars | grep quic
```

**Fixes:**
```yaml
# Increase buffers
limits:
  stream_buffer_size: 4MB
  conn_buffer_size: 50MB

# Tune QUIC
transport:
  quic:
    max_receive_buffer_size: 10MB
    max_incoming_streams: 200
```

#### High Latency
**Symptoms:** RTT much higher than direct connection

**Causes:**
- Server geographically distant
- QUIC connection migration not working
- Bufferbloat

**Fixes:**
```yaml
# Enable keep-alive
transport:
  quic:
    keep_alive: 5s

# Check for bufferbloat on path
# Consider BBR congestion control (quic-go default)
```

#### High Memory Usage
**Symptoms:** Memory grows unbounded

**Diagnosis:**
```bash
# Heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine profile
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

**Fixes:**
- Check for goroutine leaks
- Verify stream closure on client disconnect
- Reduce buffer limits

---

### 2.3 TLS/QUIC Issues

#### "no supported ALPN"
```
Error: no supported ALPN protocol
```
**Cause:** Server/client ALPN mismatch

**Fix:** Ensure both use same ALPN:
- Native QUIC: `qtunnel/1`
- HTTP/3: `h3`

#### "QUIC version negotiation failed"
```
Error: quic: version negotiation failed
```
**Cause:** QUIC version mismatch

**Fix:** Update quic-go to compatible versions on both ends.

#### "0-RTT rejected"
```
Warning: 0-RTT rejected by server
```
**Cause:** Server doesn't accept 0-RTT or replay protection

**Fix:** Disable 0-RTT in client config (default):
```yaml
transport:
  quic:
    enable_0rtt: false
```

---

### 2.4 Proxy Issues

#### "destination denied"
```
Error: connect fail: destination denied by ACL
```
**Cause:** Target matches denylist or not in allowlist

**Fix:**
```yaml
# Server config - adjust ACL
proxy:
  allowlist:
    - "api.example.com:443"
    - "10.0.0.0/8"  # If internal access needed
  denylist: []  # Or remove defaults
```

#### "connection reset by peer"
**Cause:** Target service closed connection

**Fix:** Check target service logs. Not a QTunnel issue.

#### SOCKS5 Not Working
**Cause:** Client not configured for SOCKS5

**Fix:**
```yaml
proxy:
  mode: "socks5"
  listen: "127.0.0.1:1080"
```

---

### 2.5 VPN Mode Issues (Phase 2+)

#### "TUN device creation failed"
```
Error: failed to create TUN device: permission denied
```
**Cause:** Insufficient privileges

**Fix:**
```bash
# Linux
sudo setcap cap_net_admin+ep ./qtunnel-client
# Or run as root (not recommended)

# Windows
# Run as Administrator

# macOS
sudo ./qtunnel-client ...
```

#### "routes not installed"
**Cause:** Platform route API failure

**Fix:** Check logs for specific error. May need:
- Linux: `CAP_NET_ADMIN` + `CAP_NET_RAW`
- Windows: Administrator + Wintun driver installed
- macOS: root + `networkextension` entitlement

#### DNS Leaks
**Symptom:** DNS queries bypass tunnel

**Fix:**
```yaml
vpn:
  dns:
    mode: "push"  # Push DNS to client
    servers:
      - "1.1.1.1"
      - "8.8.8.8"
  kill_switch: true  # Block non-tunnel traffic
```

---

## 3. Debugging Tools

### 3.1 Built-in Diagnostics

```bash
# Health check
curl http://localhost:8080/health

# Metrics
curl http://localhost:8080/metrics

# pprof
go tool pprof http://localhost:6060/debug/pprof/profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Expvars
curl http://localhost:6060/debug/vars
```

### 3.2 Network Capture

```bash
# QUIC packets (requires keylog)
SSLKEYLOGFILE=keylog.txt qtunnel-client -c client.yaml
# Then in Wireshark: File → Preferences → Protocols → TLS → (Pre)-Master-Secret log filename

# Or use qlog (quic-go)
qtunnel-client -c client.yaml --qlog-dir ./qlogs
```

### 3.3 Logging

```yaml
# Enable debug logging
logging:
  level: "debug"
  format: "json"  # or "console"
  output: "stderr"  # or file path
```

### 3.4 Key Log Analysis

```bash
# Filter for errors
jq 'select(.level=="error")' qtunnel.log

# Connection events
jq 'select(.event=="connection")' qtunnel.log

# Stream events
jq 'select(.event=="stream")' qtunnel.log
```

---

## 4. Platform-Specific Issues

### 4.1 Windows

| Issue | Solution |
|-------|----------|
| "The system cannot find the path specified" | Use forward slashes or double backslashes in paths |
| Wintun not found | Install Wintun driver: `wintun-install.exe` |
| Firewall blocks | Add exception for qtunnel-client.exe |

### 4.2 Linux

| Issue | Solution |
|-------|----------|
| "permission denied" on port < 1024 | Use `setcap` or authbind |
| systemd service fails | Check `journalctl -u qtunnel` |
| TUN device busy | `ip link delete tun0` (cleanup) |

### 4.3 macOS

| Issue | Solution |
|-------|----------|
| "operation not permitted" | Grant network extension permission in System Settings |
| Codesign issues | `codesign -s - ./qtunnel-client` |

---

## 5. Getting Help

### 5.1 Before Reporting

Collect:
1. `qtunnel-client version` / `qtunnel-server version`
2. Config files (redact tokens!)
3. Logs at `debug` level
4. `qtunnel-diagnose ping` output
5. Network capture if possible

### 5.2 Reporting Template

```markdown
## Environment
- OS: 
- Go version: 
- QTunnel version: 

## Config (redacted)
```yaml
# client.yaml
```

## Logs
```
# Relevant log lines
```

## Steps to Reproduce
1.
2.
3.

## Expected vs Actual
```

---

## 6. Known Issues (v0.x)

| Issue | Workaround | Fix Target |
|-------|------------|------------|
| Reconnection storms on flaky net | Increase backoff | M6 |
| Memory leak with many short-lived connections | Restart periodically | M6 |
| Windows TUN requires Admin | Run as Admin | M5 |
| No HTTP/2 fallback | Use QUIC/HTTP/3 | M8 |

---

## 7. Emergency Procedures

### 7.1 Immediate Shutdown
```bash
# Graceful (drains connections)
kill -TERM $(pgrep qtunnel)

# Force (immediate)
kill -KILL $(pgrep qtunnel)
```

### 7.2 Token Rotation (Compromise)
```bash
# 1. Generate new token
NEW_TOKEN=$(qtunnel-diagnose secret)

# 2. Update server config (add new, keep old temporarily)
# tokens: ["old", "new"]

# 3. Reload server config
kill -HUP $(pgrep qtunnel-server)

# 4. Update all clients

# 5. Remove old token from server
```

### 7.3 Certificate Rotation
```bash
# 1. Obtain new cert/key
# 2. Replace files atomically
mv new.crt server.crt
mv new.key server.key

# 3. Reload
kill -HUP $(pgrep qtunnel-server)
```

---

## 8. Log Reference

### 8.1 Log Levels
| Level | Use Case |
|-------|----------|
| debug | Detailed flow, frame dumps |
| info | Connection events, config |
| warn | Retries, near-limits |
| error | Failures, crashes |

### 8.2 Structured Log Fields

```json
{
  "time": "2026-10-25T10:30:00Z",
  "level": "info",
  "event": "connection_accepted",
  "remote_addr": "192.168.1.100:54321",
  "conn_id": "abc123",
  "transport": "quic"
}
```

| Field | Description |
|-------|-------------|
| event | Event type (connection_accepted, stream_open, auth_failed, etc.) |
| conn_id | QUIC connection ID (short) |
| stream_id | QUIC stream ID |
| remote_addr | Peer address |
| transport | quic, http3 |
| error | Error message (if level=error) |

---

## 9. FAQ

**Q: Can I run client and server on same machine?**
A: Yes, use `127.0.0.1` addresses and different ports.

**Q: Does QTunnel work through corporate proxies?**
A: HTTP/3 carrier may work if proxy supports HTTP/3. Native QUIC typically blocked.

**Q: Can I use QTunnel with Kubernetes?**
A: Yes, deploy server as Deployment/Service, client as DaemonSet or sidecar.

**Q: Is there a GUI?**
A: No, CLI only. Community contributions welcome.

**Q: How do I monitor QTunnel?**
A: Prometheus metrics at `/metrics`, health at `/health`.

**Q: Can I load balance multiple servers?**
A: Not built-in. Use external TCP/HTTP load balancer in front of servers.