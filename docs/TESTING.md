# QTunnel Testing Guide

**Version:** 1.0  
**Date:** 2026-10-25

---

## 1. Test Strategy

### 1.1 Test Pyramid

```
         ┌─────────────┐
         │   E2E /     │  Few: Full stack, real network
         │ Integration │
        ├─────────────┤
        │  Component  │  Some: Carrier, proxy, auth
        │ Integration │
       ├─────────────┤
       │    Unit     │  Many: Config, frame, session, ACL
       └─────────────┘
```

### 1.2 Test Categories

| Category | Target | Tools | Frequency |
|----------|--------|-------|-----------|
| Unit | >80% coverage | `go test`, `testify` | Every PR |
| Integration | Critical paths | In-process client/server | Every PR |
| Fuzzing | Parsers, decoders | `go test -fuzz` | Nightly + PR |
| Race Detection | Concurrency | `go test -race` | Every PR |
| Benchmarks | Performance | `go test -bench` | Release |
| Stress | Reliability | Custom framework | Pre-release |

---

## 2. Running Tests

### 2.1 Quick Commands

```bash
# All unit tests
go test ./...

# With race detector
go test -race ./...

# With coverage
go test -cover ./...

# Specific package
go test -v ./internal/config/...

# Fuzzing (30 seconds per target)
go test -fuzz=FuzzDecode -fuzztime=30s ./internal/frame

# Benchmarks
go test -bench=. -benchmem ./internal/...

# Integration tests (requires build)
go test -tags=integration ./...
```

### 2.2 Makefile Targets

```makefile
test:           # Unit tests
test-race:      # With race detector
test-cover:     # With coverage report
test-fuzz:      # Fuzzing targets
test-bench:     # Benchmarks
test-integration: # Integration tests
lint:           # golangci-lint
vet:            # go vet
fmt:            # gofmt check
```

---

## 3. Unit Test Guidelines

### 3.1 Package-Level Tests

Each package should test:
- Public API behavior
- Error conditions
- Edge cases (empty, max, boundary)
- Concurrency safety (where applicable)

### 3.2 Test Naming

```go
func TestConfig_Validate_Valid(t *testing.T)
func TestConfig_Validate_InvalidMissingToken(t *testing.T)
func TestFrame_EncodeDecode_RoundTrip(t *testing.T)
func TestSession_StreamLifecycle(t *testing.T)
```

### 3.3 Test Helpers

```go
// internal/testutil/helpers.go
func NewTestConfig() *config.Config
func NewTestServer(t *testing.T) *Server
func NewTestClient(t *testing.T) *Client
func AssertFramesEqual(t *testing.T, expected, actual []Frame)
```

---

## 4. Integration Tests

### 4.1 In-Process Client/Server

```go
func TestIntegration_TCPProxy(t *testing.T) {
    // Start server in goroutine
    server := NewTestServer(t)
    defer server.Close()
    
    // Start client
    client := NewTestClient(t)
    defer client.Close()
    
    // Dial through proxy
    conn, err := client.Dial("target:80")
    require.NoError(t, err)
    
    // Send/receive data
    _, err = conn.Write([]byte("GET / HTTP/1.1\r\n\r\n"))
    require.NoError(t, err)
    
    buf := make([]byte, 4096)
    n, err := conn.Read(buf)
    require.NoError(t, err)
    assert.Contains(t, string(buf[:n]), "HTTP/1.1")
}
```

### 4.2 Test Scenarios

| Scenario | Description |
|----------|-------------|
| Single connection | Basic proxy flow |
| Concurrent connections | 100 parallel |
| Large transfer | 100MB+ bidirectional |
| Interruption | Kill client/server mid-transfer |
| Reconnection | Client reconnects after server restart |
| TLS verification | Invalid cert rejected |
| Auth failure | Wrong token rejected |
| ACL enforcement | Denied destination rejected |

---

## 5. Fuzzing

### 5.1 Fuzz Targets

```go
// internal/frame/fuzz_test.go
func FuzzDecode(f *testing.F) {
    f.Add([]byte{0x01, 0x00, 0x02, 0x09, 0x65, 0x78})
    f.Fuzz(func(t *testing.T, data []byte) {
        _, _ = frame.Decode(data)
    })
}

func FuzzConfigParse(f *testing.F) {
    f.Add([]byte(`role: "client"\nserver:\n  addr: "x:1"`))
    f.Fuzz(func(t *testing.T, data []byte) {
        _ = config.Parse(bytes.NewReader(data))
    })
}
```

### 5.2 Running Fuzzing

```bash
# Frame decoder
go test -fuzz=FuzzDecode -fuzztime=30s ./internal/frame

# Config parser
go test -fuzz=FuzzConfigParse -fuzztime=30s ./internal/config

# Capsule parser (MASQUE)
go test -fuzz=FuzzCapsule -fuzztime=30s ./internal/carrier/masque
```

### 5.3 Corpus Management

- Store interesting inputs in `testdata/fuzz/<target>/`
- Commit corpus for regression testing
- Run `go test -fuzz=FuzzDecode -fuzztime=5m` for deep fuzzing

---

## 6. Race Detection

### 6.1 Running with Race

```bash
go test -race ./...
go test -race -count=10 ./internal/session  # Stress
```

### 6.2 Common Patterns to Avoid

| Pattern | Problem | Fix |
|---------|---------|-----|
| Map access without mutex | Race | `sync.RWMutex` |
| Slice append from multiple goroutines | Race | Channel or mutex |
| Close channel twice | Panic | `sync.Once` |
| Timer reuse | Race | New timer each use |

---

## 7. Benchmarks

### 7.1 Benchmark Structure

```go
func BenchmarkFrameEncode(b *testing.B) {
    f := Frame{Type: FrameData, Payload: bytes.Repeat([]byte("x"), 1024)}
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = f.Encode()
    }
}

func BenchmarkSessionOpenStream(b *testing.B) {
    sess := NewTestSession(b)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        stream, _ := sess.OpenStream(context.Background())
        stream.Close()
    }
}
```

### 7.2 Running Benchmarks

```bash
# All benchmarks
go test -bench=. -benchmem ./...

# Specific
go test -bench=BenchmarkFrameEncode -benchmem ./internal/frame

# Compare (save baseline)
go test -bench=. -benchmem ./... > bench.txt
# After changes
go test -bench=. -benchmem ./... | benchcmp bench.txt -
```

### 7.3 Metrics to Track

| Metric | Target |
|--------|--------|
| Frame encode/decode | < 1 µs |
| Stream open/close | < 10 µs |
| 1MB throughput | > 1 Gbps |
| Memory/connection | < 2 MB |
| Connection setup | < 50ms |

---

## 8. CI Configuration

### 8.1 GitHub Actions (`.github/workflows/ci.yml`)

```yaml
name: CI
on: [push, pull_request]

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        go: ['1.22', '1.23']
        os: [ubuntu-latest, windows-latest, macos-latest]
        exclude:
          - go: '1.22'
            os: windows-latest
          - go: '1.22'
            os: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache: true
      - run: go mod download
      - run: make test
      - run: make test-race
      - run: make lint
      - run: govulncheck ./...
  
  fuzz:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: make test-fuzz
  
  bench:
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: make test-bench
      - uses: benchmark-action/github-action-benchmark@v1
```

---

## 9. Test Data Management

### 9.1 Certificates for Testing

```bash
# Generate test certs (not for production!)
make test-certs
# Creates: testdata/certs/server.crt, server.key, ca.crt
```

### 9.2 Test Configurations

```
configs/
├── client.example.yaml
├── server.example.yaml
├── test/
│   ├── client.yaml
│   └── server.yaml
```

---

## 10. Debugging Tests

### 10.1 Verbose Output

```bash
go test -v -run TestIntegration_TCPProxy ./internal/integration
```

### 10.2 Debug Logging

```go
// In test
os.Setenv("QTUNNEL_LOG_LEVEL", "debug")
```

### 10.3 pprof During Tests

```go
// In test setup
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

---

## 11. Test Maintenance

- **Flaky tests:** Quarantine immediately, fix within 48h
- **Slow tests:** Profile, optimize, or mark as integration
- **Coverage drops:** Investigate, add tests for new code
- **Fuzzing crashes:** Fix immediately, add regression test

---

## 12. Checklist for New Code

- [ ] Unit tests for all public functions
- [ ] Error path tests
- [ ] Boundary condition tests
- [ ] Concurrency tests (if applicable)
- [ ] Fuzz target for parsers
- [ ] Benchmark for hot paths
- [ ] Integration test for new features
- [ ] Race detector clean
- [ ] Coverage maintained >80%