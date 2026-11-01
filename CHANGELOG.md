# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial project structure and documentation (M1)
- Configuration system with YAML validation and env overrides (M2)
- Authentication framework with token management (M2)
- Session state machine and stream multiplexing (M2)
- Frame protocol with varint encoding (M2)
- Observability: logging, metrics, health checks (M2)
- Native QUIC carrier implementation (M3)
- HTTP/3 carrier with Extended CONNECT support (M4)
- Carrier abstraction layer (M4)
- MASQUE CONNECT-UDP client (M5)
- MASQUE CONNECT-IP client (M5)
- Cross-platform TUN device abstraction (M5)
- VPN mode with kill switch (M5)
- Failure injection framework (M6)
- Graceful shutdown manager (M6)
- Config hot-reload (M6)
- Certificate hot-reload (M6)
- Session resumption design (M6)
- Stress testing framework (M6)
- Chaos injection for network failures (M6)

### Changed
- Refactored client to use carrier abstraction
- Updated server to handle MASQUE frames

### Security
- Constant-time token comparison
- TLS 1.3 mandatory
- Default denylist for private IPs

---

## [0.1.0-alpha] - 2026-11-15 (Planned)

### Added
- Native QUIC TCP proxy
- Basic client/server CLI
- Configuration validation
- Token authentication

---

## Version History

| Version | Date | Milestone | Notes |
|---------|------|-----------|-------|
| 0.1.0-alpha | 2026-11-15 | M3 | First working TCP proxy over QUIC |
| 0.2.0-alpha | 2026-12-15 | M4 | HTTP/3 carrier + MASQUE UDP |
| 0.3.0-alpha | 2027-01-15 | M5 | CONNECT-IP + VPN mode |
| 0.4.0-beta | 2027-02-15 | M6 | Reliability hardening |
| 0.5.0-beta | 2027-03-15 | M7 | Quality assurance |
| 1.0.0 | 2027-04-15 | M8 | Production release |