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
  labels:
    {{- range $k, $v := .Spec.Labels }}
    {{ $k }}: "{{ $v }}"
    {{- end }}
  name: {{ .Spec.ControlPlane.WebApp.SvcName }}
  namespace: {{ ns . }}
spec:
{{- if ne .Spec.Networking.Ingress.ClassName "" }}
  ingressClassName: {{ .Spec.Networking.Ingress.ClassName }}
{{- end }}
  rules:
  - host: "{{ .Spec.ControlPlane.WebApp.SvcName }}.{{ .Spec.ClusterDomain }}"
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: {{ .Spec.ControlPlane.WebApp.SvcName }}
            port:
              number: {{ .Spec.ControlPlane.WebApp.Port }}
{{- if isTrue .Spec.Networking.HTTPS.Enabled }}
  tls:
  - hosts:
    - {{ .Spec.ControlPlane.WebApp.SvcName }}.{{ .Spec.ClusterDomain }}
{{- if isTrue .Spec.Networking.HTTPS.AcmeCert }}
    secretName: app-tls-cert
{{- else if ne .Spec.Networking.HTTPS.CertSecret "" }}
    secretName: {{ .Spec.Networking.HTTPS.CertSecret }}
{{- end }}
{{- end }}
