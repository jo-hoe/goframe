# goframe

[![Lint](https://github.com/jo-hoe/goframe/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/jo-hoe/goframe/actions/workflows/lint.yml)
[![Test](https://github.com/jo-hoe/goframe/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/jo-hoe/goframe/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jo-hoe/goframe)](https://goreportcard.com/report/github.com/jo-hoe/goframe)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/goframe/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/goframe?branch=main)

Image processing web service written in Go. The services is used for e-ink photo frames that display a different image each day from a curated set.

The service provides a web UI to upload and manage images, applies a configurable processing pipeline to each image, and serves the currently scheduled image via an API endpoint. Images are rotated daily based on a timezone-aware schedule.

## Supported commands and parameters

Configure the processing pipeline via the `commands` section in `config.yaml`.

Refer to `config.example.yaml` for a full example including a custom palette for dithering.

## Configuration

The server reads configuration from a YAML file:

- Default path: `./config.yaml` (in current working directory)
- Override via environment: `CONFIG_PATH=/path/to/config.yaml`

Required and optional fields:

- port: int (server port, default 8080)
- database:
  - type: string (e.g. "redis")
  - connectionString: string (e.g. "localhost:6379")
  - namespace: string (key namespace, e.g. "goframe")
- timezone: string (IANA TZ, default "UTC") — used for daily image rotation and CronJob scheduling
- commands: list of command definitions (see above)

## Quick start (local)

Prerequisites:

- Go 1.25+
- Redis (or use `make start-docker` to run via docker-compose)

Steps:

1. Copy `config.example.yaml` to `config.yaml` and adjust as needed
2. Run the server:
   - go run ./cmd/server
   - or build first: `go build ./...` then run the built binary
3. Open the UI: <http://localhost:8080/>
   - Upload an image
   - See current image thumbnail and the schedule list
   - Delete images if needed

API test:

- Health: `curl http://localhost:8080/probe`
- Current processed image (PNG): `curl -s http://localhost:8080/api/image.png -o current.png`
- Upload an image (multipart/form-data, field "image"): `curl -s -X POST -F "image=@/path/to/your/image.png" http://localhost:8080/api/image`
- List images (IDs and URLs): `curl http://localhost:8080/api/images`
- Download processed by ID: `curl -s http://localhost:8080/api/images/<id>/processed.png -o processed.png`
- Download original by ID: `curl -s http://localhost:8080/api/images/<id>/original.png -o original.png`
- Delete by ID: `curl -X DELETE http://localhost:8080/api/images/<id> -i`

## Helm

The chart is located in `charts/goframe`. It depends on the [Bitnami Redis chart](https://charts.bitnami.com/bitnami), which must be fetched before installing:

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm dependency build charts/goframe
helm install goframe charts/goframe
```

## Redis sizing

Images are stored in Redis as base64-encoded strings (original + processed). Use this rule of thumb to size `maxmemory`:

```
maxmemory ≈ image_count × avg_original_size_MB × 1.5
```

Example: 100 images with a 3 MB average original → ~450 MB → set `maxmemory` to 512mb.

Set the container memory limit about 20% above `maxmemory` to leave room for Redis overhead (e.g. 512mb maxmemory → 640Mi container limit).

Recommended flags:

```
--maxmemory <size>
--maxmemory-policy allkeys-lru
```

With `allkeys-lru`, Redis evicts the least-recently-used images when memory is full instead of returning an error. If you prefer hard failures over silent eviction, use `noeviction` and size `maxmemory` generously.

## Make

Use `make help` to see available targets.
