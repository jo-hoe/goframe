# syntax=docker/dockerfile:1

# ---------- Build stage ----------
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

# Tools and certs for HTTPS and optional UPX compression
RUN apk add --no-cache ca-certificates upx

# We can build a fully static binary (modernc.org/sqlite is CGO-free)
ENV CGO_ENABLED=0

# Leverage build cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source
COPY . .

# Build the backend binary
RUN mkdir -p /out \
    && go build -trimpath -ldflags="-s -w" -o /out/goframe ./cmd/server

RUN upx --lzma --best /out/goframe || true

# ---------- Runtime stage ----------
# Switch to a Python-based runtime since we need to execute a Python dithering script
FROM python:3.11-alpine AS runner

# CA certs for outbound HTTPS if needed
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the backend binary and scripts
COPY --from=builder /out/goframe /app/goframe
COPY scripts/ /app/scripts/

# Install Python dependencies
RUN pip install --no-cache-dir pillow

# Run as non-root
RUN addgroup -S app && adduser -S -G app app && chown -R app:app /app
USER app

ENTRYPOINT ["/app/goframe"]
