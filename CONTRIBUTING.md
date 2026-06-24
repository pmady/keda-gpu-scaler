# Contributing to keda-gpu-scaler

Contributions are welcome.

# Contributors

See CONTRIBUTORS.md for a list of project contributors and their contributions.

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

### Pre-commit Hooks

This repository ships a [pre-commit](https://pre-commit.com) configuration
(`.pre-commit-config.yaml`) that catches trailing whitespace, missing final
newlines, and Go formatting issues before you push. Install it once:

```bash
pip install pre-commit   # or: brew install pre-commit
pre-commit install       # set up the git hook in your clone
```

The hooks then run automatically on every `git commit`. To run them against the
whole tree on demand:

```bash
pre-commit run --all-files
```

`golangci-lint` is configured as a `manual` hook (it can be slow), so it does
not run on every commit. Invoke it explicitly when you want it:

```bash
pre-commit run --hook-stage manual golangci-lint
```

> [!NOTE]
> The compiled binaries (`keda-gpu-scaler` and `gpu-metrics`) dynamically link NVIDIA's NVML library (`libnvidia-ml.so`) at runtime. **They will fail to start on any machine that does not have the NVIDIA driver installed** — for example, a laptop or CI runner with no NVIDIA GPU. You can still build, lint, and run the full test suite without a GPU; all tests use a mock collector.

### Testing on a GPU Cluster

The binary requires NVIDIA GPU drivers (`libnvidia-ml.so`) to run. For local development without GPUs, unit tests cover all parsing, aggregation, and metric extraction logic.

## Cutting a Release

Releases are fully automated via GitHub Actions. To publish a new release:

1. Ensure all changes are merged to `main`.
2. Push a semver version tag (`vX.Y.Z` — the release workflow triggers on `v*`, so
   make sure the tag is valid semver to avoid a broken release):
   ```bash
   git tag v0.5.0
   git push origin v0.5.0
   ```
3. The [release workflow](https://github.com/pmady/keda-gpu-scaler/actions/workflows/release.yaml) will:
   - Build and push multi-arch Docker images (`linux/amd64`, `linux/arm64`) to GHCR
   - Compile release binaries for both architectures
   - Create a GitHub Release with an auto-generated changelog and attach the binaries

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
