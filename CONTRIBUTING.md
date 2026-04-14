# Contributing to keda-gpu-scaler

Contributions are welcome.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-user>/keda-gpu-scaler.git`
3. Create a feature branch: `git checkout -b feat/my-feature`
4. Make your changes
5. Run tests: `make test`
6. Commit with sign-off: `git commit -s -m "feat: my feature"`
7. Push and open a Pull Request

## Development

### Prerequisites

- Go 1.23+
- `protoc` (for regenerating protobuf code)
- Docker (for building container images)
- Helm 3 (for chart development)

### Build & Test

```bash
make build    # Build binary (requires CGO)
make test     # Run unit tests
make lint     # Run golangci-lint
make proto    # Regenerate protobuf Go code
```

### Testing on a GPU Cluster

The binary requires NVIDIA GPUs and drivers to run. For local development without GPUs, unit tests cover all parsing, aggregation, and metric extraction logic.

## Code Style

- Follow standard Go conventions
- Use `go fmt` and `golangci-lint`
- Write table-driven tests
- Sign off all commits (`git commit -s`)

## Reporting Issues

Please use GitHub Issues. Include:
- What you expected to happen
- What actually happened
- Steps to reproduce
- GPU hardware and driver version
- KEDA version

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
