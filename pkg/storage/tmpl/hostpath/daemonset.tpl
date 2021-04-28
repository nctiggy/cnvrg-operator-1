apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: hostpath-provisioner
  namespace: {{ ns . }}
  labels:
    k8s-app: hostpath-provisioner
spec:
  selector:
    matchLabels:
      k8s-app: hostpath-provisioner
  template:
    metadata:
      labels:
        k8s-app: hostpath-provisioner
    spec:
      {{- if isTrue .Spec.Tenancy.Enabled }}
      nodeSelector:
        {{ .Spec.Tenancy.Key }}: {{ .Spec.Tenancy.Value }}
        {{- range $key, $val := .Spec.Storage.Hostpath.NodeSelector }}
        {{ $key }}: {{ $val }}
      {{- end }}
      tolerations:
        - key: "{{ .Spec.Tenancy.Key }}"
          operator: "Equal"
          value: "{{ .Spec.Tenancy.Value }}"
          effect: "NoSchedule"
      {{- else if (gt (len .Spec.Storage.Hostpath.NodeSelector) 0) }}
      nodeSelector:
        {{- range $key, $val := .Spec.Storage.Hostpath.NodeSelector }}
        {{ $key }}: {{ $val }}
        {{- end }}
      {{- end }}
      serviceAccountName: hostpath-provisioner-admin
      containers:
        - name: hostpath-provisioner
          image: {{ .Spec.Storage.Hostpath.Image }}
          imagePullPolicy: Always
          env:
            - name: USE_NAMING_PREFIX
              value: "true" # change to true, to have the name of the pvc be part of the directory
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: PV_DIR
              value: {{ .Spec.Storage.Hostpath.Path }}
          volumeMounts:
            - name: pv-volume # root dir where your bind mounts will be on the node
              mountPath: {{ .Spec.Storage.Hostpath.Path }}
          resources:
            limits:
              cpu: {{ .Spec.Storage.Hostpath.CPULimit }}
              memory: {{ .Spec.Storage.Hostpath.MemoryLimit }}
            requests:
              cpu: {{ .Spec.Storage.Hostpath.CPURequest }}
              memory: {{ .Spec.Storage.Hostpath.MemoryRequest }}
      volumes:
        - name: pv-volume
          hostPath:
            path: {{ .Spec.Storage.Hostpath.Path }}
