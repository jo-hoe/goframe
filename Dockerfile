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
# Distroless static nonroot keeps image very small and secure
FROM gcr.io/distroless/static:nonroot AS runner

# Copy CA certs for outbound HTTPS if needed
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /app

# Copy the binary
COPY --from=builder /out/goframe /app/goframe

USER nonroot:nonroot

ENTRYPOINT ["/app/goframe"]
