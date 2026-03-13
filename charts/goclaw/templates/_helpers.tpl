{{/*
Expand the name of the chart.
*/}}
{{- define "goclaw.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "goclaw.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "goclaw.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "goclaw.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "goclaw.selectorLabels" -}}
app.kubernetes.io/name: {{ include "goclaw.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
DB host — internal if db.enabled, else externalDatabase.host
*/}}
{{- define "goclaw.dbHost" -}}
{{- if .Values.db.enabled }}
{{- printf "%s-db" (include "goclaw.fullname" .) }}
{{- else }}
{{- .Values.externalDatabase.host }}
{{- end }}
{{- end }}

{{/*
DB port
*/}}
{{- define "goclaw.dbPort" -}}
{{- if .Values.db.enabled }}
{{- "5432" }}
{{- else }}
{{- .Values.externalDatabase.port | toString }}
{{- end }}
{{- end }}

{{/*
DB name
*/}}
{{- define "goclaw.dbName" -}}
{{- if .Values.db.enabled }}
{{- "goclaw" }}
{{- else }}
{{- .Values.externalDatabase.name }}
{{- end }}
{{- end }}

{{/*
DB user
*/}}
{{- define "goclaw.dbUser" -}}
{{- if .Values.db.enabled }}
{{- "goclaw" }}
{{- else }}
{{- .Values.externalDatabase.user }}
{{- end }}
{{- end }}

{{/*
Secret name — either existingSecret or auto-generated
*/}}
{{- define "goclaw.secretName" -}}
{{- if .Values.config.existingSecret }}
{{- .Values.config.existingSecret }}
{{- else }}
{{- include "goclaw.fullname" . }}
{{- end }}
{{- end }}
