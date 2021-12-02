apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: cnvrg-buildimage-job
  namespace: {{ ns . }}
  annotations:
    {{- range $k, $v := .Spec.Annotations }}
    {{$k}}: "{{$v}}"
    {{- end }}
  labels:
    {{- range $k, $v := .Spec.Labels }}
    {{$k}}: "{{$v}}"
    {{- end }}
{{- if not .Spec.ControlPlane.BaseConfig.CnvrgJobRbacStrict }}
rules:
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - '*'
- apiGroups:
  - ""
  resources:
  - services
  verbs:
  - '*'
{{- end }}