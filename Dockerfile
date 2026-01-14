# syntax=docker/dockerfile:1

# ---------- Go Build stage ----------
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine AS go_builder

WORKDIR /src

# Tools and certs for HTTPS
RUN apk add --no-cache ca-certificates

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

# ---------- Node CLI build stage ----------
FROM node:20-bullseye-slim AS node_builder

WORKDIR /cli

# Install minimal build deps for node-canvas (PNG-only: Cairo + libpng via libcairo2-dev)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    python3 \
    pkg-config \
    libcairo2-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy CLI sources
COPY tools/epdoptimize-cli/package.json /cli/package.json

# Install production deps (build native addon for canvas)
RUN if [ -f package-lock.json ]; then npm ci --omit=dev; else npm install --omit=dev; fi
COPY tools/epdoptimize-cli/index.mjs /cli/index.mjs

# ---------- Runtime stage ----------
# Use Debian slim so node + Cairo runtime libs are available
FROM node:20-bullseye-slim AS runner

WORKDIR /app

# Install minimal runtime libs for node-canvas (PNG-only)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libcairo2 \
    && rm -rf /var/lib/apt/lists/*

# Copy CA certs for outbound HTTPS if needed (from base image)
# Node image already has certs, but keep path consistent
# No extra step needed here.

# Copy the Go binary
COPY --chown=node:node --from=go_builder /out/goframe /app/goframe

# Copy the CLI (node_modules + index.mjs)
COPY --chown=node:node --from=node_builder /cli /app/tools/epdoptimize-cli

# Ensure working dir is writable for SQLite (data.db will be created under /app)
RUN chown -R node:node /app

USER node

ENTRYPOINT ["/app/goframe"]
