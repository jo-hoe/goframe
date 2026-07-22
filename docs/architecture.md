# GoFrame Kubernetes Architecture

## Overview

GoFrame is a Kubernetes-native image rotation system. A custom operator manages the lifecycle of all components: a stateless web server, S3-compatible storage (RustFS), and CronJob-based image schedulers.

```mermaid
graph TD
    subgraph "Control Plane"
        OP[GoFrame Operator]
        CR[GoFrame CR]
    end

    subgraph "Data Plane"
        SRV[GoFrame Server]
        RUSTFS[RustFS StatefulSet]
    end

    subgraph "Scheduling"
        CJ1[CronJob: Scheduler 1]
        CJ2[CronJob: Scheduler N]
    end

    subgraph "Networking"
        ING_SRV[Ingress: Server]
        ING_IMG[Ingress: Images]
        MW[Traefik Middleware]
    end

    CR -->|watched by| OP
    OP -->|reconciles| SRV
    OP -->|reconciles| RUSTFS
    OP -->|reconciles| CJ1
    OP -->|reconciles| CJ2

    SRV -->|stores blobs + metadata| RUSTFS

    CJ1 -->|POST /api/image| SRV
    CJ2 -->|POST /api/image| SRV

    ING_SRV -->|/* routes| SRV
    ING_IMG -->|/images routes| RUSTFS
    MW -->|rewrites path| ING_IMG
```

---

## Component Details

| Component | Kind | Port | Purpose |
|-----------|------|------|---------|
| GoFrame Operator | Deployment | 8082/8083 | Reconciles GoFrame CRs |
| GoFrame Server | Deployment | 8080 | Web UI, API, image processing |
| RustFS | StatefulSet | 9000 | S3-compatible blob + metadata storage |
| Image Schedulers | CronJob (one per source) | - | Fetch images from external sources |
| Server Ingress | Ingress | 80/443 | Routes UI/API traffic |
| RustFS Ingress | Ingress + Middleware | 80/443 | Direct browser access to images |

---

## Data Flow: Image Upload

```mermaid
sequenceDiagram
    participant B as Browser
    participant I as Ingress
    participant S as GoFrame Server
    participant R as RustFS

    B->>I: POST /api/image (multipart)
    I->>S: forward request
    S->>S: Process image pipeline<br/>(rotate, scale, crop, dither)
    S->>R: PUT images/{id}/original.png
    S->>R: PUT images/{id}/processed.png
    S->>R: PUT rotation.json<br/>(update ordered_ids + images map)
    S-->>B: 200 {"id": "..."}
```

---

## Data Flow: Image Display

```mermaid
sequenceDiagram
    participant B as Browser
    participant I as Ingress
    participant S as GoFrame Server
    participant MW as Traefik Middleware
    participant R as RustFS

    B->>I: GET /api/image.png
    I->>S: forward to server
    S->>R: Read rotation.json (ordered_ids[0])
    S-->>B: 302 /images/{id}/processed.png

    B->>I: GET /images/{id}/processed.png
    I->>MW: Match /images prefix
    MW->>MW: Rewrite path:<br/>/images/* → /{bucket}/images/*
    MW->>R: GET /{bucket}/images/{id}/processed.png
    R-->>B: 200 (PNG bytes, direct)
```

---

## Ingress Routing

```mermaid
graph LR
    Browser -->|"GET /*"| Traefik[Traefik Ingress Controller]

    Traefik -->|"path: /images/*"| RustFS_Ingress[RustFS Ingress]
    Traefik -->|"path: /*"| Server_Ingress[Server Ingress]

    RustFS_Ingress -->|"rewrite: /{bucket}/images/*"| RustFS[RustFS :9000]
    Server_Ingress --> Server[GoFrame Server :8080]
```

The RustFS ingress uses a Traefik `replacePathRegex` Middleware to rewrite `/images/(.*)` to `/{bucket}/images/$1`, mapping the browser-facing URL to the S3 object key.

A bucket policy grants anonymous `s3:GetObject` on `images/*`, so no authentication is required for image fetches through the ingress.

---

## Operator Reconciliation

```mermaid
graph TD
    CR[GoFrame CR] --> R[Reconcile Loop]

    R --> RS[reconcileServer]
    R --> RC[reconcileCronJobs]
    R --> RR[reconcileRotation]
    R --> US[updateStatus]

    RS --> CM1[ConfigMap: server config]
    RS --> DEP[Deployment: server]
    RS --> SVC[Service: server]
    RS --> RUSTFS_SS[StatefulSet: rustfs]
    RS --> RUSTFS_SVC[Service: rustfs]
    RS --> SEC[Secret: rustfs-credentials]

    RC --> |per scheduler| SCM[ConfigMap: scheduler config]
    RC --> |per scheduler| SCRON[CronJob: scheduler]
    RC --> |if s3 source| SSEC[Secret: s3-credentials]

    RR --> |read/write| ROT[rotation.json in RustFS]
    RR --> |advance at midnight| ORDER[Rotate ordered_ids]

    US --> STATUS[CR Status: currentImageID,<br/>lastRotationTime, serverReady]
```

---

## Storage: rotation.json

All state is stored in RustFS as `rotation.json`. The server is stateless — no PVC or local database is required.

```json
{
  "last_rotated": "2026-05-31T00:00:00Z",
  "ordered_ids": ["id-b", "id-a"],
  "images": {
    "id-a": { "created_at": "2026-05-30T10:00:00Z", "source": "xkcd" },
    "id-b": { "created_at": "2026-05-31T09:00:00Z", "source": "" }
  }
}
```

- `ordered_ids`: display order; index 0 is today's image
- `images`: per-image metadata (creation time, source label)
- `last_rotated`: timestamp of the last midnight rotation by the operator

Both the server and operator read and write this file. The server owns all image CRUD writes; the operator advances `ordered_ids` and updates `last_rotated` at midnight.

---

## Scheduler Architecture

```mermaid
graph TD
    subgraph "Sources"
        XKCD[XKCD API]
        MET[Met Museum API]
        TUMBLR[Tumblr RSS]
        S3SRC[S3 Bucket]
        NASAAPOD[NASA APOD API]
        NASAIOTD[NASA Image of the Day RSS]
    end

    subgraph "CronJobs (one per scheduler)"
        CJ[Image Scheduler Pod]
    end

    CJ -->|fetch random image| XKCD
    CJ -->|fetch random image| MET
    CJ -->|fetch random image| TUMBLR
    CJ -->|fetch random image| S3SRC
    CJ -->|fetch random image| NASAAPOD
    CJ -->|fetch latest image| NASAIOTD

    CJ -->|"POST /api/image<br/>(with source label)"| SRV[GoFrame Server]

    SRV -->|"if group:<br/>delete other group members' images"| SRV
```

Each scheduler is configured with:
- **cron**: When to run (timezone-aware)
- **source**: Which image source to use (`xkcd`, `oatmeal`, `metmuseum`, `tumblr`, `s3`, `nasaapod`, `nasaimageoftheday`)
- **group**: Mutually exclusive scheduling (e.g., weekday vs weekend)
- **onExternalImages**: Policy for non-group images (`ignore`, `takeover`, `yield`)
- **commands**: Optional per-scheduler image processing pipeline

### NASA sources

| Source | Fetch behaviour | Config |
|---|---|---|
| `nasaapod` | Picks a random entry from the full APOD archive via `api.nasa.gov` | Optional `apiKeySecretRef` for a production API key |
| `nasaimageoftheday` | Fetches the latest image from the NASA RSS feed (`nasa.gov/feed/`) | No additional configuration required |

---

## Rotation Logic

The operator performs timezone-aware midnight rotation:

1. Read `rotation.json` from RustFS (`ordered_ids`, `last_rotated`)
2. Compare current day (in configured timezone) to `last_rotated` day
3. If new day: rotate `ordered_ids` by number of elapsed days
4. Write updated `ordered_ids` and `last_rotated` back to `rotation.json`
5. Requeue reconciliation for next midnight

The server reads `rotation.json` on each request and serves `ordered_ids[0]` as the current image, ensuring operator and server stay in sync without direct communication.
