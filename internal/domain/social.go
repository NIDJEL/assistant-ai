package domain

// SocialNotice links a persisted chat message to an authorized live view. Event
// details and invitation buttons are fetched with the current user's session.
type SocialNotice struct {
	Kind    string `json:"kind"`
	EventID int64  `json:"event_id,omitempty"`
}
