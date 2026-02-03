# goframe

![Version: 3.1.0](https://img.shields.io/badge/Version-3.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 3.1.0](https://img.shields.io/badge/AppVersion-3.1.0-informational?style=flat-square)

Helm chart for the goframe image processing web service

## Source Code

* <https://github.com/jo-hoe/goframe>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for Pod scheduling |
| config | object | `{"commands":[{"name":"OrientationCommand","orientation":"portrait"},{"name":"DitherCommand"}],"database":{"connectionString":"file:goframe.db?cache=shared&mode=rwc","type":"sqlite"},"logLevel":"info","port":8080,"rotationTimezone":"UTC","svgFallbackLongSidePixelCount":4096,"thumbnailWidth":512}` | Application configuration rendered into config.yaml and mounted into the container |
| config.commands | list | `[{"name":"OrientationCommand","orientation":"portrait"},{"name":"DitherCommand"}]` | Processing pipeline configuration Supported command names and parameters: - OrientationCommand: orientation [portrait|landscape], rotateWhenSquare (bool, default false), clockwise (bool; true=clockwise, false=counterclockwise; default true) - ScaleCommand: height (int), width (int), edgeGradient (bool, optional; default false) - PixelScaleCommand: height (int, optional), width (int, optional) - at least one must be provided - CropCommand: height (int), width (int) - PngConverterCommand: no parameters; enforces PNG output - DitherCommand: palette (list of device/dither pairs), e.g. [[[0,0,0],[25,30,33]], [[255,255,255],[232,232,232]]] Examples (uncomment to use): - name: ScaleCommand   height: 1920   width: 1080   edgeGradient: false - name: PixelScaleCommand   width: 1080 - name: CropCommand   height: 1600   width: 1200 - name: PngConverterCommand - name: DitherCommand   palette:     - [[0, 0, 0],[25, 30, 33]]     - [[255, 255, 255],[232, 232, 232]] |
| config.database | object | `{"connectionString":"file:goframe.db?cache=shared&mode=rwc","type":"sqlite"}` | Database configuration |
| config.database.connectionString | string | `"file:goframe.db?cache=shared&mode=rwc"` | Connection string (':memory:' for in-memory SQLite) |
| config.database.type | string | `"sqlite"` | Database driver (e.g., sqlite) |
| config.logLevel | string | `"info"` | Log level for application (debug, info, warn, error) |
| config.port | int | `8080` | Port of the application |
| config.rotationTimezone | string | `"UTC"` | Timezone used for image rotation scheduling |
| config.svgFallbackLongSidePixelCount | int | `4096` | Fallback long-side pixel count used when rendering SVGs without explicit width/height Aspect ratio is retained using viewBox when available; falls back to square otherwise |
| config.thumbnailWidth | int | `512` | Thumbnail width for thumbnails in the frontend |
| configRaw | string | `""` |  |
| extraEnv | list | `[]` | Extra environment variables to inject into the container |
| extraEnvFrom | list | `[]` | Extra environment sources (e.g., ConfigMaps or Secrets) |
| fullnameOverride | string | `""` | Fully override the release name |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/jo-hoe/goframe"` | Container image repository |
| image.tag | string | `""` | Overrides the image tag whose default is the chart appVersion. |
| imagePullSecrets | list | `[]` | Secrets to use for pulling images (for private registries) |
| ingress.annotations | object | `{}` | Annotations to add to the Ingress |
| ingress.className | string | `""` | IngressClass name |
| ingress.enabled | bool | `false` | Enable Ingress |
| ingress.hosts | list | `[{"host":"goframe.local","paths":[{"path":"/","pathType":"Prefix"}]}]` | Ingress host definitions |
| ingress.tls | list | `[]` | TLS configuration for the Ingress |
| nameOverride | string | `""` | Partially override the chart name |
| nodeSelector | object | `{}` | Node selector for Pod assignment |
| podAnnotations | object | `{}` | Annotations to add to the Pod |
| podLabels | object | `{}` | Additional labels to add to the Pod |
| podSecurityContext | object | `{}` | Pod-level security context |
| replicaCount | int | `1` | Number of desired pod replicas |
| resources | object | `{}` | Resource requests and limits for the container |
| securityContext | object | `{}` | Container-level security context |
| service.port | int | `80` | Service port |
| service.targetPort | int | `8080` | Target container port exposed by the application |
| service.type | string | `"ClusterIP"` | Kubernetes Service type |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.automount | bool | `true` | Automatically mount a ServiceAccount's API credentials |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.name | string | `""` | The name of the service account to use. If not set and create is true, a name is generated using the fullname template |
| tolerations | list | `[]` | Tolerations for Pod assignment |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
