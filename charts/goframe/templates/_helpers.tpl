{{/* Helpers for the goframe instance chart (renders only a GoFrame CR). */}}

{{/*
Common labels
*/}}
{{- define "goframe.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/name: {{ .Release.Name | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
{{- end }}
