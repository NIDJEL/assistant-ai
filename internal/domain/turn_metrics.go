package domain

type TurnMetrics struct {
	SearchCalls  int     `json:"search_calls"`
	ModelCalls   int     `json:"model_calls"`
	Repairs      int     `json:"repairs"`
	PlanSteps    int     `json:"plan_steps"`
	WaitingSteps int     `json:"waiting_steps"`
	StateBytes   int     `json:"state_bytes"`
	InferenceMS  float64 `json:"inference_ms"`
	DurationMS   float64 `json:"duration_ms"`
}
