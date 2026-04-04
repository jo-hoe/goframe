{{/*
Expand the name of the chart.
*/}}
{{- define "goframe-operator.name" -}}
{{- .Chart.Name }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "goframe-operator.labels" -}}
app.kubernetes.io/name: {{ include "goframe-operator.name" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}
