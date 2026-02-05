# goframe

[![Lint](https://github.com/jo-hoe/goframe/actions/workflows/lint.yml/badge.svg?branch=main)](https://github.com/jo-hoe/goframe/actions/workflows/lint.yml)
[![Test](https://github.com/jo-hoe/goframe/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/jo-hoe/goframe/actions/workflows/test.yml)

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
  - type: string (e.g. "sqlite")
  - connectionString: string (e.g. "file:goframe.db?cache=shared&mode=rwc" or ":memory:")
- rotationTimezone: string (IANA TZ, default "UTC")
- commands: list of command definitions (see above)

## Quick start (local)

Prerequisites:

- Go 1.24+
- SQLite (embedded via modernc.org/sqlite, no external DB needed)

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

## Make

Use `make help` to see available targets.
