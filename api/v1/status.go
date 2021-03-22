package v1

type OperatorStatus string

const (
	StatusError       OperatorStatus = "ERROR"
	StatusReconciling OperatorStatus = "RECONCILING"
	StatusHealthy     OperatorStatus = "HEALTHY"
	StatusReady       OperatorStatus = "READY"
	StatusRemoving    OperatorStatus = "REMOVING"
)

type Status struct {
	Status   OperatorStatus `json:"status,omitempty"`
	Message  string         `json:"message,omitempty"`
	Progress string         `json:"progress,omitempty"`
}
