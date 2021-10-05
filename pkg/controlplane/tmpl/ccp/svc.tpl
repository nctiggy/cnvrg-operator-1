apiVersion: v1
kind: Service
metadata:
  labels:
    control-plane: controller-manager
  name: cnvrg-ccp-operator-controller-manager-metrics-service
  namespace: {{ ns . }}
spec:
  ports:
    - name: https
      port: 8443
      targetPort: https
  selector:
    control-plane: controller-manager