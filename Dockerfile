# syntax=docker/dockerfile:1
# Builds any of the three goframe binaries from a single Dockerfile.
# Select which binary to build via the CMD build arg:
#   docker build --build-arg CMD=server        -t goframe .
#   docker build --build-arg CMD=imagescheduler -t goframe-image-scheduler .
#   docker build --build-arg CMD=operator       -t goframe-operator .
# The resulting binary is always placed at /app/goframe regardless of CMD.

ARG GO_VERSION=1.25
# CMD selects which subdirectory under cmd/ to build (server, imagescheduler, operator).
ARG CMD=server

# ---------- Build stage ----------
# Full Go toolchain + Alpine tools. Compiles the binary and compresses it with upx.
FROM golang:${GO_VERSION}-alpine AS builder
# Re-declare ARG after FROM so it is visible inside this stage.
ARG CMD

WORKDIR /src

# ca-certificates: needed so the binary can make TLS calls at runtime (copied to runner stage).
# upx: compresses the binary to reduce image size.
RUN apk add --no-cache ca-certificates upx

# Disable cgo so the binary is fully static and can run in distroless/scratch images.
ENV CGO_ENABLED=0

# Download dependencies before copying source so this layer is cached when only source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# -trimpath: removes local file paths from the binary (reproducibility + security).
# -ldflags="-s -w": strips debug info and DWARF tables to shrink binary size.
RUN mkdir -p /out \
    && go build -trimpath -ldflags="-s -w" -o /out/goframe ./cmd/${CMD}

# Compress with upx; "|| true" so the build doesn't fail if upx can't handle the binary.
RUN upx --lzma --best /out/goframe || true

# ---------- Dev stage ----------
# Used by docker-compose for local development (target: dev in docker-compose.yml).
# Based on Alpine (not distroless) because the goframe healthcheck uses wget, which
# distroless does not provide. Do not use this stage in production.
FROM golang:${GO_VERSION}-alpine AS dev
COPY --from=builder /out/goframe /app/goframe
ENTRYPOINT ["/app/goframe"]

# ---------- Runtime stage ----------
# Minimal distroless image for production. No shell, no package manager — only the binary
# and CA certificates. Runs as a non-root user for least-privilege security.
FROM gcr.io/distroless/static:nonroot AS runner

# Copy CA certificates from the builder so the binary can verify TLS connections.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /app

COPY --from=builder /out/goframe /app/goframe

USER nonroot:nonroot

ENTRYPOINT ["/app/goframe"]
