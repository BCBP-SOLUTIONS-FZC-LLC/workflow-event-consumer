{{- define "workflow-events-consumer.name" -}}
workflow-events-consumer
{{- end -}}

{{- define "workflow-events-consumer.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- .Release.Name -}}-{{- include "workflow-events-consumer.name" . -}}
{{- end -}}
{{- end -}}

{{- define "workflow-events-consumer.labels" -}}
app.kubernetes.io/name: {{ include "workflow-events-consumer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "workflow-events-consumer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "workflow-events-consumer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "workflow-events-consumer.serviceAccountName" -}}
{{- $fixed := include "workflow-events-consumer.fullname" . -}}
{{- if and .Values.serviceAccount.name (ne .Values.serviceAccount.name $fixed) -}}
{{- fail (printf "serviceAccount.name is %q, but the Definition, Execution and Stream Gateway charts admit this service by the ServiceAccount %q (workflow-app/workflow-events-consumer in their callers lists). Leave serviceAccount.name empty, or change every caller list first." .Values.serviceAccount.name $fixed) -}}
{{- end -}}
{{- $fixed -}}
{{- end -}}
