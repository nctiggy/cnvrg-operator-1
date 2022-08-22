apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    nginx.ingress.kubernetes.io/proxy-send-timeout: 18000s
    nginx.ingress.kubernetes.io/proxy-read-timeout: 18000s
    nginx.ingress.kubernetes.io/proxy-body-size: 5G
    {{- range $k, $v := .Spec.Annotations }}
    {{ $k }}: "{{ $v }}"
    {{- end }}
  name: {{ .Spec.Logging.ElastAlert.SvcName }}
  namespace: {{ ns . }}
  labels:
    {{- range $k, $v := .Spec.Labels }}
    {{ $k }}: "{{ $v }}"
    {{- end }}
spec:
  rules:
    - host: "{{ .Spec.Logging.ElastAlert.SvcName }}.{{ .Spec.ClusterDomain }}"
      http:
        paths:
          - path: /
            backend:
              service
                name: {{ .Spec.Logging.ElastAlert.SvcName }}
                port: 
                  number: {{ .Spec.Logging.ElastAlert.Port }}