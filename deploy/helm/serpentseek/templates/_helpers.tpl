{{/*
Expand the name of the chart.
*/}}
{{- define "serpentseek.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully qualified app name.
*/}}
{{- define "serpentseek.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Chart label (name-version).
*/}}
{{- define "serpentseek.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels.
*/}}
{{- define "serpentseek.labels" -}}
helm.sh/chart: {{ include "serpentseek.chart" . }}
{{ include "serpentseek.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels.
*/}}
{{- define "serpentseek.selectorLabels" -}}
app.kubernetes.io/name: {{ include "serpentseek.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Name of the generated secret holding envSecrets.
*/}}
{{- define "serpentseek.envSecretName" -}}
{{- printf "%s-env" (include "serpentseek.fullname" .) -}}
{{- end -}}

{{/*
Name of the data PVC.
*/}}
{{- define "serpentseek.pvcName" -}}
{{- printf "%s-data" (include "serpentseek.fullname" .) -}}
{{- end -}}

{{/*
Whether the pod uses the SQLite data volume (absent with an external database).
*/}}
{{- define "serpentseek.usesDataVolume" -}}
{{- if and .Values.persistence.enabled (not .Values.externalDatabase.enabled) -}}true{{- end -}}
{{- end -}}

{{/*
The image reference.
*/}}
{{- define "serpentseek.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) -}}
{{- end -}}
