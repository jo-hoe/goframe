# syntax=docker/dockerfile:1

# ---------- Go Build stage ----------
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS go_builder

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
    && go build -trimpath -ldflags="-s -w" -o /out/goframe ./cmd/server \
    && upx --lzma --best /out/goframe || true

# ---------- Node CLI build stage ----------
FROM node:20-bullseye AS node_builder

WORKDIR /cli

# Install build deps for node-canvas
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    python3 \
    pkg-config \
    libcairo2-dev \
    libpango1.0-dev \
    libjpeg-dev \
    libgif-dev \
    librsvg2-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy CLI sources
COPY tools/epdoptimize-cli/package.json /cli/package.json
COPY tools/epdoptimize-cli/index.mjs /cli/index.mjs

# Install production deps (build native addon for canvas)
RUN npm install --omit=dev

# ---------- Runtime stage ----------
# Use Debian slim so node + Cairo runtime libs are available
FROM node:20-bullseye-slim AS runner

WORKDIR /app

# Install runtime libs for node-canvas
RUN apt-get update && apt-get install -y --no-install-recommends \
    libcairo2 \
    libpango-1.0-0 \
    libjpeg62-turbo \
    libgif7 \
    librsvg2-2 \
    && rm -rf /var/lib/apt/lists/*

# Copy CA certs for outbound HTTPS if needed (from base image)
# Node image already has certs, but keep path consistent
# No extra step needed here.

# Copy the Go binary
COPY --from=go_builder /out/goframe /app/goframe

# Copy the CLI (node_modules + index.mjs)
RUN mkdir -p /app/tools/epdoptimize-cli
COPY --from=node_builder /cli /app/tools/epdoptimize-cli

# Default user (root) to allow writing temp files if needed
# USER node

ENTRYPOINT ["/app/goframe"]
