package domain

type SearchStatus struct {
	Calls   int    `json:"calls,omitempty"`
	Mode    string `json:"mode"`
	Model   string `json:"model,omitempty"`
	Warning string `json:"warning,omitempty"`
}
