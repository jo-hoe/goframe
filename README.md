# goframe

[![Lint](https://github.com/jo-hoe/goframe/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/jo-hoe/goframe/actions/workflows/lint.yml)
[![Test](https://github.com/jo-hoe/goframe/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/jo-hoe/goframe/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/jo-hoe/goframe)](https://goreportcard.com/report/github.com/jo-hoe/goframe)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/goframe/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/goframe?branch=main)

Image processing web service written in Go. The service is used for e-ink photo frames that display a different image each day from a curated set.

The service provides a web UI to upload and manage images, applies a configurable processing pipeline to each image, and serves the currently scheduled image via an API endpoint. Images are rotated daily based on a timezone-aware schedule.

## Architecture

Images are stored in SeaweedFS (S3-compatible object storage). Metadata and rotation state are stored alongside blobs in SeaweedFS as `rotation.json` — no local database or PVC required. The server is stateless. The server returns 302 redirects to SeaweedFS URLs for image delivery.

A Kubernetes operator manages:
- SeaweedFS (StatefulSet + Service + Secret)
- goframe server (Deployment + Service + ConfigMap)
- CronJob per scheduler entry

See [docs/architecture.md](docs/architecture.md) for detailed diagrams.

## Configuration

The server reads configuration from a YAML file:

- Default path: `./local.yaml` (in current working directory)
- Override via environment: `CONFIG_PATH=/path/to/config.yaml`

See `local.example.yaml` for all available fields.

## Quick start (local)

Prerequisites:

- Go 1.26+
- Docker (for SeaweedFS via docker-compose)

Steps:

1. Copy `local.example.yaml` to `local.yaml` and adjust as needed
2. Start SeaweedFS: `make start-docker`
3. Run the server: `go run ./cmd/server`
4. Open the UI: <http://localhost:8080/>

API test:

- Health: `curl http://localhost:8080/probe`
- Current processed image (PNG): `curl -s http://localhost:8080/api/image.png -o current.png`
- Upload an image: `curl -s -X POST -F "image=@/path/to/image.png" http://localhost:8080/api/image`
- List images: `curl http://localhost:8080/api/images`
- Delete by ID: `curl -X DELETE http://localhost:8080/api/images/<id> -i`

## Helm

The chart is located in `charts/goframe`. Install with:

```bash
helm dependency build charts/goframe
helm install goframe charts/goframe
```

The operator chart (`charts/goframe-operator`) must be installed first to register the GoFrame CRD.

## Schedulers

CronJob-based image schedulers fetch images from external sources automatically.

Supported sources: `xkcd`, `oatmeal`, `metmuseum`, `tumblr`, `s3`

Key configuration:
- **group**: Schedulers sharing a group evict each other's images on upload (mutual exclusion)
- **onExternalImages**: Policy when non-group images exist (`ignore`, `takeover`, `yield`)

Each scheduler always keeps exactly one image per source.

See `charts/goframe/values.yaml` for full examples.

## Make

Use `make help` to see available targets.
