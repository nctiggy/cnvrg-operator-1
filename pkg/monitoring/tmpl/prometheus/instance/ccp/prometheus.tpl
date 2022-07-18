apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: cnvrg-ccp-prometheus
  namespace: {{ ns . }}
  annotations:
    {{- range $k, $v := .Spec.Annotations }}
    {{$k}}: "{{$v}}"
    {{- end }}
  labels:
    {{- range $k, $v := .Spec.Labels }}
    {{$k}}: "{{$v}}"
    {{- end }}
spec:
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          topologyKey: kubernetes.io/hostname
          labelSelector:
            matchExpressions:
            - key: cnvrg
              operator: In
              values:
              - {{ .Spec.Monitoring.Prometheus.SvcName }}
  storage:
    disableMountSubPath: true
    volumeClaimTemplate:
      spec:
        resources:
          requests:
            storage: {{ .Spec.Monitoring.Prometheus.StorageSize }}
        {{- if ne .Spec.Monitoring.Prometheus.StorageClass "" }}
        storageClassName: {{ .Spec.Monitoring.Prometheus.StorageClass }}
        {{- end }}
  image: {{ image .Spec.ImageHub .Spec.Monitoring.Prometheus.Image }}
  replicas: {{ .Spec.Monitoring.Prometheus.Replicas }}
  retention: 8w # 2 months
  retentionSize: {{ promRetentionSize .Spec.Monitoring.Prometheus.StorageSize }} # total PVC size - 2 Gi
  podMetadata:
    {{- if .Spec.Annotations }}
    annotations:
      {{- range $k, $v := .Spec.Annotations }}
      {{$k}}: "{{$v}}"
      {{- end }}
    {{- end }}
    labels:
      cnvrg: {{ .Spec.Monitoring.Prometheus.SvcName }}
      {{- range $k, $v := .Spec.Labels }}
      {{$k}}: "{{$v}}"
      {{- end }}
  resources:
    requests:
      cpu: {{ .Spec.Monitoring.Prometheus.Requests.Cpu }}
      memory: {{ .Spec.Monitoring.Prometheus.Requests.Memory }}
    limits:
      cpu: {{ .Spec.Monitoring.Prometheus.Limits.Cpu }}
      memory: {{ .Spec.Monitoring.Prometheus.Limits.Memory }}
  ruleSelector:
    matchLabels:
      app: cnvrg-ccp-prometheus
      role: alert-rules
  securityContext:
    fsGroup: 2000
    runAsNonRoot: true
    runAsUser: 1000
  serviceAccountName: cnvrg-ccp-prometheus
  priorityClassName: {{ .Spec.CnvrgAppPriorityClass.Name }}
  podMonitorNamespaceSelector: {}
  podMonitorSelector: {}
  probeNamespaceSelector: {}
  serviceMonitorNamespaceSelector: {}
  serviceMonitorSelector:
    matchLabels:
      cnvrg-ccp-prometheus: {{ .Name }}-{{ ns .}}
  version: v2.22.1
  additionalScrapeConfigs:
    name: {{ .Spec.Monitoring.Prometheus.UpstreamRef }}
    key: prometheus-additional.yaml
  listenLocal: true
  containers:
    - name: "prom-auth-proxy"
      image: {{ image .Spec.ImageHub .Spec.Monitoring.Prometheus.BasicAuthProxyImage }}
      resources:
        requests:
          cpu: 100m
          memory: 100Mi
        limits:
          cpu: 1000m
          memory: 1Gi
      ports:
        - containerPort: 9091
          name: web
      volumeMounts:
        - name: "prom-auth-proxy"
          mountPath: "/etc/nginx"
          readOnly: true
        - name: "htpasswd"
          mountPath: "/etc/nginx/htpasswd"
          readOnly: true
  volumes:
    - name: "prom-auth-proxy"
      configMap:
        name: "prom-auth-proxy"
    - name: "htpasswd"
      secret:
        secretName: {{ .Spec.Monitoring.Prometheus.CredsRef }}
  {{- if isTrue .Spec.Tenancy.Enabled }}
  nodeSelector:
    {{ .Spec.Tenancy.Key }}: {{ .Spec.Tenancy.Value }}
    {{- range $key, $val := .Spec.Dbs.Es.NodeSelector }}
    {{ $key }}: {{ $val }}
    {{- end }}
  tolerations:
    - operator: "Exists"
  {{- else if (gt (len .Spec.Dbs.Es.NodeSelector) 0) }}
  nodeSelector:
    {{- range $key, $val := .Spec.Dbs.Es.NodeSelector }}
    {{ $key }}: {{ $val }}
    {{- end }}
  {{- end }}
