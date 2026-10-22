{{- define "workflow-events-consumer.name" -}}
workflow-events-consumer
{{- end -}}

{{- define "workflow-events-consumer.fullname" -}}
{{- .Release.Name -}}-{{- include "workflow-events-consumer.name" . -}}
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
{{- if .Values.serviceAccount.create -}}
{{- default (include "workflow-events-consumer.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "workflow-events-consumer.secretName" -}}
{{- if .Values.existingSecret -}}
{{- .Values.existingSecret -}}
{{- else -}}
{{- include "workflow-events-consumer.fullname" . -}}
{{- end -}}
{{- end -}}
