# Contributing to QTunnel

Thank you for your interest in contributing to QTunnel! This document outlines the process and standards for contributing.

## Code of Conduct

This project adheres to a Code of Conduct. By participating, you are expected to uphold this code. For any questions, feedback, or suggestions, please leave a comment or open a new issue on GitHub.

## Getting Started

### Development Environment

1. **Fork** the repository
2. **Clone** your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/qtunnel
   cd qtunnel
   ```
3. **Install dependencies:**
   ```bash
   make deps
   ```
4. **Generate test certificates:**
   ```bash
   make certs
   ```
5. **Run tests:**
   ```bash
   make test
   ```

### Prerequisites

- Go 1.22+ (matches quic-go version policy)
- `golangci-lint` (for linting)
- `govulncheck` (for vulnerability scanning)
- `openssl` (for test certificates)

Install tools:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

## Development Workflow

### 1. Create an Issue

Before starting work, create or comment on an issue to discuss the change. This avoids duplicate effort and ensures alignment.

### 2. Create a Branch

```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

### 3. Make Changes

Follow the coding standards below. Write tests for new functionality.

### 4. Run Quality Checks

```bash
make fmt          # Check formatting
make vet          # Run go vet
make lint         # Run golangci-lint
make test         # Run unit tests
make test-race    # Run with race detector
make test-fuzz    # Run fuzz tests (quick)
```

All checks must pass before submitting.

### 5. Commit

Write clear, conventional commit messages:

```
feat(config): add environment variable override support

- Add EnvPrefix to config loader
- Support QTTUNNEL_ prefix for all settings
- Update documentation

Closes #123
```

### 6. Push and Create PR

```bash
git push origin feature/your-feature-name
```

Open a Pull Request against `main` with:
- Clear description of changes
- Reference to related issue
- Test results (if applicable)
- Screenshots for UI changes

## Coding Standards

### Go Style

- Follow [Effective Go](https://go.dev/doc/effective_go)
- Use `gofmt` formatting (enforced by CI)
- Run `golangci-lint` - all warnings must be addressed
- Prefer standard library over external dependencies

### Package Organization

```
internal/
  config/       # Configuration parsing/validation
  auth/         # Authentication (tokens, mTLS)
  session/      # Session state machine
  frame/        # Frame encoding/decoding
  carrier/      # Transport carriers
    quic/       # Native QUIC
    http3/      # HTTP/3
    masque/     # MASQUE (CONNECT-UDP, CONNECT-IP)
  client/       # Client implementation
  server/       # Server implementation
  proxy/        # Proxy handlers (TCP, SOCKS5)
  observability/ # Logging, metrics, tracing
  chaos/        # Failure injection
  recovery/     # Graceful shutdown, config reload
  stress/       # Stress testing
```

### Naming Conventions

| Element | Convention |
|---------|------------|
| Packages | Lowercase, single word (`config`, `session`) |
| Types | PascalCase (`Config`, `SessionManager`) |
| Functions/Methods | PascalCase (`Parse`, `OpenStream`) |
| Private | camelCase (`internalField`, `privateMethod`) |
| Constants | PascalCase (`DefaultTimeout`) |
| Interfaces | Noun or `-er` suffix (`Carrier`, `Stream`) |

### Error Handling

```go
// Good: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to dial %s: %w", addr, err)
}

// Good: Sentinel errors for expected conditions
var ErrInvalidToken = errors.New("invalid authentication token")

// Avoid: Panic in library code (only in main/init)
```

### Testing

- **Unit tests:** Every public function
- **Error paths:** Test all error returns
- **Boundaries:** Zero, max, overflow values
- **Concurrency:** Race detector must pass
- **Fuzzing:** All parsers/decoders

```go
func TestConfig_Validate(t *testing.T) {
    tests := []struct {
        name    string
        cfg     *Config
        wantErr bool
    }{
        {"valid", validConfig(), false},
        {"missing token", noTokenConfig(), true},
        {"invalid port", invalidPortConfig(), true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.cfg.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Documentation

- **Package comments:** Every package has a doc comment
- **Exported types/functions:** Godoc comments
- **Complex logic:** Inline comments explaining *why*, not *what*
- **README updates:** For user-facing changes

```go
// Config holds the application configuration.
// It is loaded from YAML and validated before use.
type Config struct {
    // Role is either "client" or "server".
    Role string `yaml:"role"`
    ...
}
```

## Pull Request Requirements

### Before Submitting

- [ ] `make fmt` passes (no formatting changes needed)
- [ ] `make vet` passes
- [ ] `make lint` passes
- [ ] `make test` passes
- [ ] `make test-race` passes
- [ ] New code has tests (>80% coverage target)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated (for user-facing changes)

### PR Template

```markdown
## Description
Brief description of changes.

## Related Issue
Closes #XXX

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update
- [ ] Refactoring

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing performed (describe)

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Comments added for complex logic
- [ ] Documentation updated
- [ ] No new warnings from linter
```

## Review Process

1. **Automated checks** run on every PR (CI)
2. **Maintainer review** - at least one approval required
3. **Security review** - for auth/crypto/network changes
4. **Merge** - squash and merge to main

## Release Process

Releases are managed by maintainers:

1. Update version in `main.go`
2. Update `CHANGELOG.md`
3. Create git tag: `git tag v1.0.0`
4. Push tag: `git push origin v1.0.0`
5. GitHub Actions builds and publishes release

## Security Issues

**DO NOT** report security vulnerabilities via public issues or PRs.

For any questions, feedback, or suggestions, please leave a comment or open a new issue on GitHub.

See [SECURITY.md](SECURITY.md) for details.

## Questions?

- Open a [GitHub Discussion](https://github.com/qtunnel/qtunnel/discussions)
- Check existing [issues](https://github.com/qtunnel/qtunnel/issues)
- Review [documentation](docs/)

---

Thank you for contributing to QTunnel!
