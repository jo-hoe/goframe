# goframe

![Version: 13.0.2](https://img.shields.io/badge/Version-13.0.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 13.0.2](https://img.shields.io/badge/AppVersion-13.0.2-informational?style=flat-square)

Helm chart for the goframe image processing web service

## Source Code

* <https://github.com/jo-hoe/goframe>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| commands | list | `[]` | Image processing pipeline applied to every ingested image. Supported commands: OrientationCommand, ScaleCommand, PixelScaleCommand,                     CropCommand, PngConverterCommand, DitherCommand Examples (uncomment to use): commands:   - name: OrientationCommand     params:       orientation: portrait   - name: ScaleCommand     params:       height: 1920       width: 1080   - name: DitherCommand     params:       ditheringAlgorithm: atkinson       palette:         - [[0, 0, 0],[25, 30, 33]]         - [[255, 255, 255],[232, 232, 232]] |
| ingress.annotations | object | `{}` | Annotations for the goframe Ingress resource |
| ingress.className | string | `""` | IngressClass name. Empty = cluster default. |
| ingress.enabled | bool | `true` | Enable Kubernetes Ingress for the goframe server |
| ingress.hosts | list | `[{"paths":[{"path":"/","pathType":"Prefix"}]}]` | Ingress host rules |
| ingress.tls | list | `[]` | TLS configuration for the goframe Ingress |
| schedulerImage | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/jo-hoe/goframe-image-scheduler","tag":""}` | Default image used for scheduler CronJobs when no per-scheduler image is specified |
| schedulerImage.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the scheduler container |
| schedulerImage.repository | string | `"ghcr.io/jo-hoe/goframe-image-scheduler"` | Scheduler container image repository |
| schedulerImage.tag | string | `""` | Scheduler image tag. Defaults to the chart appVersion when empty. |
| schedulers | list | `[]` | CronJob-based image schedulers (one CronJob per entry). Each entry requires: name, cron, source. Supported sources: xkcd, oatmeal, metmuseum, tumblr, s3.  group: optional. Schedulers sharing the same group evict each other's images on a successful upload, so only one group member's image is displayed at a time. Use this to stagger different sources across days or time periods.  onExternalImages: optional (default: ignore). Controls what happens when images not owned by this scheduler or any group member exist:   ignore   — upload normally, leave external images untouched   takeover — delete all external images after uploading   yield    — delete own images, skip upload  Example — weekday/weekend stagger: schedulers:   - name: weekday-xkcd     cron: "0 8 * * 1-5"     source: xkcd     group: daily-wallpaper     onExternalImages: takeover   - name: weekend-tumblr     cron: "0 8 * * 6,0"     source: tumblr     group: daily-wallpaper     onExternalImages: ignore     tumblr:       blogs:         - pusheen  Example — tumblr blog: schedulers:   - name: nasa-tumblr     cron: "0 8 * * *"     source: tumblr     tumblr:       blogs:         - nasa         # blog name without .tumblr.com         - pusheen      # add more blogs to pick from randomly  Example — metmuseum with department filter: schedulers:   - name: met-daily     cron: "0 8 * * *"     source: metmuseum     metmuseum:       # departmentIDs is optional — omit to search all departments.       # Available IDs:       #   1=American Decorative Arts  3=Ancient West Asian Art  4=Arms and Armor       #   5=Arts of Africa, Oceania, and the Americas           6=Asian Art       #   7=The Cloisters             8=The Costume Institute   9=Drawings and Prints       #   10=Egyptian Art             11=European Paintings     12=European Sculpture       #   13=Greek and Roman Art      14=Islamic Art            15=Robert Lehman Collection       #   17=Medieval Art             19=Photographs            21=Modern Art       departmentIDs:         - 6   # Asian Art         - 9   # Drawings and Prints         - 11  # European Paintings  Example — s3-compatible source (AWS S3, SeaweedFS, MinIO): schedulers:   - name: s3-daily     cron: "0 8 * * *"     source: s3     s3:       endpoint: "https://s3.us-east-1.amazonaws.com"  # or "http://seaweedfs:9000" for SeaweedFS       bucket: "my-images"       prefix: "wallpapers/"   # optional; omit to use all objects in the bucket.                                # listing is fully recursive — sub-sub-folders are included                                # automatically (no S3 delimiter is used).       region: "us-east-1"     # any non-empty value for SeaweedFS       # secretRef names a Kubernetes Secret with keys "accessKey" and "secretKey".       # Omit for anonymous access to public buckets.       secretRef: "my-s3-credentials"  Example — single source with per-scheduler processing pipeline: schedulers:   - name: xkcd     cron: "0 8 * * *"     source: xkcd     commands:       - name: ScaleCommand         height: 1600         width: 1200     image:       repository: ghcr.io/jo-hoe/goframe-image-scheduler       tag: ""       pullPolicy: IfNotPresent  |
| seaweedfs.address | string | `""` | Base URL of an external S3-compatible endpoint (e.g. "http://my-s3:9000"). Empty = use bundled SeaweedFS. |
| seaweedfs.bucket | string | `""` | S3 bucket name. Defaults to the Helm release name when empty. |
| seaweedfs.credentials.accessKey | string | `""` | SeaweedFS access key. Auto-generated when empty. |
| seaweedfs.credentials.secretKey | string | `""` | SeaweedFS secret key. Auto-generated when empty. |
| seaweedfs.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the SeaweedFS container |
| seaweedfs.image.repository | string | `"chrislusf/seaweedfs"` | SeaweedFS container image repository |
| seaweedfs.image.tag | string | `"latest"` | SeaweedFS container image tag |
| seaweedfs.imageBaseURL | string | `""` | Browser-facing URL prefix for served images. Defaults to "/images" when empty. |
| seaweedfs.ingress.annotations | object | `{}` | Annotations for the SeaweedFS Ingress resource |
| seaweedfs.ingress.className | string | `""` | IngressClass name for SeaweedFS ingress. Empty = cluster default. |
| seaweedfs.ingress.enabled | bool | `true` | Expose SeaweedFS images via Kubernetes Ingress for direct browser access |
| seaweedfs.persistence.enabled | bool | `true` | Enable persistent storage for SeaweedFS |
| seaweedfs.persistence.size | string | `"1Gi"` | Persistent volume size. Scale with image_count × avg_size_MB × 1.5. |
| seaweedfs.persistence.storageClass | string | `""` | StorageClass for the persistent volume. Empty = cluster default. |
| seaweedfs.resources | object | `{"limits":{"cpu":"500m","memory":"512Mi"},"requests":{"cpu":"50m","memory":"128Mi"}}` | Resource requests and limits for the SeaweedFS pod. `weed server -s3` runs master + volume + filer + S3 gateway in one process; ~100 images and light traffic fit comfortably in the defaults. |
| seaweedfs.secretRef | string | `""` | Name of a Kubernetes Secret with keys `accessKey` and `secretKey`. Auto-created when empty. |
| server.image.pullPolicy | string | `"IfNotPresent"` | Image pull policy for the goframe server container |
| server.image.repository | string | `"ghcr.io/jo-hoe/goframe"` | goframe server container image repository |
| server.image.tag | string | `""` | goframe server image tag. Defaults to the chart appVersion when empty. |
| server.logLevel | string | `"info"` | Log verbosity level (debug, info, warn, error) |
| server.port | int | `8080` | Port the goframe server listens on |
| server.serviceType | string | `"ClusterIP"` | How the server Service is exposed. Valid values: ClusterIP, NodePort, LoadBalancer. |
| server.svgFallbackLongSidePixelCount | int | `4096` | Long-side pixel count used when converting SVGs to raster fallbacks |
| server.thumbnailWidth | int | `512` | Width in pixels for generated thumbnails |
| timezone | string | `"UTC"` | Timezone for image rotation midnight detection (IANA timezone name) |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
