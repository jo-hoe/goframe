# GoFrame Kubernetes Architecture

## Overview

GoFrame is a Kubernetes-native image rotation system. A custom operator manages the lifecycle of all components: a web server, S3-compatible storage (RustFS), SQLite with WAL replication (Litestream), and CronJob-based image schedulers.

```mermaid
graph TD
    subgraph "Control Plane"
        OP[GoFrame Operator]
        CR[GoFrame CR]
    end

    subgraph "Data Plane"
        SRV[GoFrame Server]
        LS[Litestream Sidecar]
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

    SRV -->|stores blobs| RUSTFS
    SRV -->|reads/writes SQLite| SQLite[(SQLite /data/goframe.db)]
    LS -->|replicates WAL| RUSTFS

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
| GoFrame Server | Deployment (2 containers) | 8080 | Web UI, API, image processing |
| Litestream | Sidecar in Server Pod | - | SQLite WAL replication to RustFS |
| RustFS | StatefulSet | 9000 | S3-compatible blob storage |
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
    participant L as Litestream

    B->>I: POST /api/image (multipart)
    I->>S: forward request
    S->>S: Process image pipeline<br/>(rotate, scale, crop, dither)
    S->>R: PUT images/{id}/original.png
    S->>R: PUT images/{id}/processed.png
    S->>S: INSERT INTO SQLite<br/>(id, rank, source, created_at)
    S->>R: PUT rotation.json<br/>(update ordered_ids)
    S-->>B: 200 {"id": "..."}
    L->>R: Replicate SQLite WAL<br/>(async, continuous)
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
    S->>R: Read rotation.json (current_id)
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
    RS --> CM2[ConfigMap: litestream config]
    RS --> PVC[PVC: server-data]
    RS --> DEP[Deployment: server + litestream]
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

## Litestream Replication

```mermaid
graph LR
    subgraph "Server Pod (shared PVC)"
        SERVER[GoFrame Server] -->|writes| DB[(SQLite<br/>/data/goframe.db)]
        DB -->|WAL changes| LITESTREAM[Litestream Sidecar]
    end

    LITESTREAM -->|"s3://{bucket}/litestream/"| RUSTFS[RustFS]
```

SQLite runs in WAL mode (`PRAGMA journal_mode=WAL`) for Litestream compatibility. The sidecar continuously monitors WAL changes and replicates snapshots to RustFS. On pod restart, Litestream can restore the database from the latest snapshot.

---

## Scheduler Architecture

```mermaid
graph TD
    subgraph "Sources"
        XKCD[XKCD API]
        DA[DeviantArt RSS]
        MET[Met Museum API]
        TUMBLR[Tumblr RSS]
        S3SRC[S3 Bucket]
    end

    subgraph "CronJobs (one per scheduler)"
        CJ[Image Scheduler Pod]
    end

    CJ -->|fetch random image| XKCD
    CJ -->|fetch random image| DA
    CJ -->|fetch random image| MET
    CJ -->|fetch random image| TUMBLR
    CJ -->|fetch random image| S3SRC

    CJ -->|"POST /api/image<br/>(with source label)"| SRV[GoFrame Server]

    SRV -->|"if exclusionGroup:<br/>delete other group members' images"| SRV
```

Each scheduler is configured with:
- **cron**: When to run (timezone-aware)
- **source**: Which image source to use
- **keepCount**: Maximum images to retain from this source
- **exclusionGroup**: Mutually exclusive scheduling (e.g., weekday vs weekend)
- **commands**: Optional per-scheduler image processing pipeline

---

## Rotation Logic

The operator performs timezone-aware midnight rotation:

1. Read `rotation.json` from RustFS (`ordered_ids`, `current_id`, `last_rotated`)
2. Compare current day (in configured timezone) to `last_rotated` day
3. If new day: rotate `ordered_ids` by number of elapsed days
4. Write updated state back to `rotation.json`
5. Requeue reconciliation for next midnight

The server reads `rotation.json` on each request to determine which image to display, ensuring operator and server stay in sync without direct communication.
