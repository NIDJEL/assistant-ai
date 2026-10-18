package domain

import (
	"net/url"
	"strings"
	"time"
)

// ZoomMeeting is a public chat attachment. It never contains host credentials.
type ZoomMeeting struct {
	EventID     int64      `json:"event_id"`
	Title       string     `json:"title"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	Timezone    string     `json:"timezone"`
	Status      string     `json:"status"`
	JoinURL     string     `json:"join_url,omitempty"`
	RunID       string     `json:"run_id,omitempty"`
}

func ValidZoomJoinURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	trusted := host == "zoom.us" || strings.HasSuffix(host, ".zoom.us") || host == "zoomgov.com" || strings.HasSuffix(host, ".zoomgov.com")
	// The host-only /s/ and /start/ URLs must never become chat actions.
	path := strings.Split(strings.Trim(u.Path, "/"), "/")
	return trusted && len(path) == 2 && (path[0] == "j" || path[0] == "w") && path[1] != "" && path[1] != "." && path[1] != ".." && !u.Query().Has("zak")
}

func ZoomCard(e Event, loc *time.Location) *ZoomMeeting {
	if e.Kind != EventMeeting || e.ConferenceProvider != "zoom" {
		return nil
	}
	card := &ZoomMeeting{EventID: e.ID, Title: e.Title, Timezone: loc.String(), Status: "scheduled"}
	if start, _, ok := e.Interval(loc); ok {
		card.ScheduledAt = &start
	}
	if ValidZoomJoinURL(e.MeetingURL) {
		card.JoinURL = e.MeetingURL
	}
	return card
}
