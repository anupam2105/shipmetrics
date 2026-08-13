{{/*
Expand the name of the chart.
*/}}
{{- define "shipmetrics.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Fully-qualified app name. Truncated to 63 chars to satisfy DNS-1123.
*/}}
{{- define "shipmetrics.fullname" -}}
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
Chart label — used by the chart itself.
*/}}
{{- define "shipmetrics.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels shared by every resource.
*/}}
{{- define "shipmetrics.labels" -}}
helm.sh/chart: {{ include "shipmetrics.chart" . }}
{{ include "shipmetrics.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{/*
Selector labels — a narrow subset that never changes across upgrades.
*/}}
{{- define "shipmetrics.selectorLabels" -}}
app.kubernetes.io/name: {{ include "shipmetrics.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Name of the service account to use.
*/}}
{{- define "shipmetrics.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "shipmetrics.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Container image reference — tag defaults to Chart.AppVersion when empty.
*/}}
{{- define "shipmetrics.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

{{/*
Assert that a database source is configured — plain URL or existing secret.
*/}}
{{- define "shipmetrics.assertDatabase" -}}
{{- if and (not .Values.database.url) (not .Values.database.existingSecret) -}}
{{- fail "database.url OR database.existingSecret must be set — shipmetrics needs a PostgreSQL DSN" -}}
{{- end -}}
{{- end -}}
