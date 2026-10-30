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

{{- define "sealane.sessionSecretName" -}}
{{- $ss := default dict .Values.sessionSecret -}}
{{- if $ss.existingSecret -}}{{ $ss.existingSecret }}{{- else -}}{{ printf "%s-session" (include "sealane.fullname" .) }}{{- end -}}
{{- end -}}

{{/*
The ServiceAccount's ClusterRole. Every cluster call runs as this ServiceAccount and the app role
(admin/editor/viewer) decides what a user may do, so it needs CRUD on all managed kinds. In
impersonation mode writes run as sealane:admin / sealane:editor instead, so it only reads them, and
templates/impersonation.yaml renders it: the impersonation.yaml release asset, applied after
install.yaml, replaces install.yaml's role that can write.
*/}}
{{- define "sealane.clusterRole" -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "sealane.fullname" . }}
  labels:
    {{- include "sealane.labels" . | nindent 4 }}
rules:
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["get", "list"]
  # Managed resources (same groups as internal/k8s/registry.go)
  - apiGroups:
      - networking.istio.io
      - security.istio.io
      - telemetry.istio.io
      - gateway.networking.k8s.io
    resources: ["*"]
    {{- if (default dict .Values.impersonation).enabled }}
    verbs: ["get", "list", "watch"]   # impersonation mode: writes run as sealane:admin / sealane:editor
    {{- else }}
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
    {{- end }}
  # Reads for form dropdowns / namespace picker, ready endpoint counts for the flow map,
  # and pods for the analysis page (subset and gateway selectors that match nothing).
  - apiGroups: [""]
    resources: ["namespaces", "services", "endpoints", "pods"]
    verbs: ["get", "list"]
  {{- if (default dict .Values.requestTester).enabled }}
  # Request tester (opt-in): run curl inside a pod. exec is effectively arbitrary command execution, so it is only granted when requestTester.enabled=true.
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]
  {{- end }}
{{- end }}
