# Benchmark Results Documentation

**Date:** TBD (Run after Go 1.22+ installation)
**Go Version:** TBD
**Platform:** TBD (Linux/Windows/macOS)
**Hardware:** TBD

---

## Benchmark Methodology

Each benchmark runs for 3 seconds (`-benchtime=3s`) with memory allocation tracking (`-benchmem`).

```bash
go test -bench=. -benchmem -benchtime=3s ./internal/...
```

---

## Frame Package Benchmarks

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkFrameEncode | TBD | TBD | TBD | TBD |
| BenchmarkFrameDecode | TBD | TBD | TBD | TBD |
| BenchmarkVarintEncode | TBD | TBD | TBD | TBD |
| BenchmarkVarintDecode | TBD | TBD | TBD | TBD |

### Expected Targets
- Frame encode/decode: < 1 µs per operation
- Varint encode/decode: < 50 ns per operation

---

## Session Package Benchmarks

| Benchmark | Iterations | ns/op | B/op | allocs/op |
|-----------|------------|-------|------|-----------|
| BenchmarkSessionOpenStream | TBD | TBD | TBD | TBD |
| BenchmarkSessionStreamCount | TBD | TBD | TBD | TBD |
| BenchmarkSessionManagerAddRemove | TBD | TBD | TBD | TBD |
| BenchmarkFlowControl | TBD | TBD | TBD | TBD |

### Expected Targets
- Stream open/close: < 10 µs
- Stream count: < 50 ns
- Flow control (1KB): < 1 µs

---

## Integration Benchmarks (Requires Running Server)

| Benchmark | Configuration | Target |
|-----------|---------------|--------|
| QUIC Throughput | Local loopback, 1 stream | > 1 Gbps |
| QUIC Throughput | Local loopback, 100 streams | > 5 Gbps |
| HTTP/3 Throughput | Local loopback, 1 stream | > 800 Mbps |
| Connection Establishment | Local loopback, TLS 1.3 | < 50 ms (p99) |
| Connection Establishment | Local loopback, TLS 1.3 resumption | < 10 ms (p99) |

---

## MASQUE Benchmarks

| Benchmark | Configuration | Target |
|-----------|---------------|--------|
| CONNECT-UDP Throughput | Local loopback, 1 stream | > 500 Mbps |
| CONNECT-IP Throughput | Local loopback, 1 stream | > 500 Mbps |
| UDP Latency Overhead | vs direct UDP | < 1 ms |
| IP Packet Latency Overhead | vs direct IP | < 2 ms |

---

## Memory Benchmarks

| Metric | Idle (100 conns) | Active (100 conns, 100 streams each) | Target |
|--------|------------------|--------------------------------------|--------|
| Heap Alloc | TBD | TBD | < 50 MB |
| Heap Sys | TBD | TBD | < 100 MB |
| Goroutines | TBD | TBD | < 5000 |
| GC Pause (p99) | TBD | TBD | < 5 ms |

---

## Recording Results

After running benchmarks, update this file with actual numbers:

```bash
# Run all benchmarks and save
go test -bench=. -benchmem -benchtime=3s ./internal/... > bench_results.txt

# Compare with previous run
go test -bench=. -benchmem -benchtime=3s ./internal/... | benchcmp bench_results.txt bench_results_new.txt
```

---

## Historical Results

| Date | Go Version | Platform | Key Changes | Status |
|------|------------|----------|-------------|--------|
| TBD | TBD | TBD | Initial baseline | 🟡 Pending |

---

## Notes

- Run benchmarks on dedicated hardware for consistent results
- Disable CPU frequency scaling: `cpupower frequency-set -g performance`
- Pin to specific CPU cores: `taskset -c 0-3`
- Run multiple times and take median