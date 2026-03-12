# Build stage
FROM golang:1.25-bookworm AS builder

# Install build dependencies for CGO (required by go-nvml)
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o /keda-gpu-scaler ./cmd/keda-gpu-scaler/

# Runtime stage — uses NVIDIA base image for libnvidia-ml.so
FROM nvidia/cuda:12.6.3-base-ubuntu24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /keda-gpu-scaler /usr/local/bin/keda-gpu-scaler

USER 65534:65534

ENTRYPOINT ["/usr/local/bin/keda-gpu-scaler"]
