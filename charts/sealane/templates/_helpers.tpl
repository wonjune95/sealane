{{- define "sealane.name" -}}{{ .Chart.Name }}{{- end -}}

{{- define "sealane.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "sealane.labels" -}}
app.kubernetes.io/name: {{ include "sealane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "sealane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "sealane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
