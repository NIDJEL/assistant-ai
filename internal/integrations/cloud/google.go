package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

type googleTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}
type googleEvent struct {
	Transparency string     `json:"transparency,omitempty"`
	ID           string     `json:"id,omitempty"`
	ETag         string     `json:"etag,omitempty"`
	Updated      string     `json:"updated,omitempty"`
	Status       string     `json:"status,omitempty"`
	Summary      string     `json:"summary"`
	Description  string     `json:"description,omitempty"`
	Location     string     `json:"location,omitempty"`
	HangoutLink  string     `json:"hangoutLink,omitempty"`
	Start        googleTime `json:"start"`
	End          googleTime `json:"end"`
	Extended     struct {
		Private map[string]string `json:"private,omitempty"`
	} `json:"extendedProperties,omitempty"`
}

func (s *Service) googleURL(c *repository.CloudConnection) string {
	return s.GoogleBase + "/calendars/" + url.PathEscape(c.CalendarID) + "/events"
}
func (s *Service) fromGoogle(g googleEvent) (*domain.Event, error) {
	title := strings.TrimSpace(g.Summary)
	if title == "" {
		title = "Без названия"
	}
	r := []rune(title)
	if len(r) > 500 {
		title = string(r[:500])
	}
	e := &domain.Event{Kind: domain.EventTask, Title: title, Description: g.Description, Location: g.Location, MeetingURL: g.HangoutLink, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityNormal}
	e.CalendarProvider = "google"
	e.GoogleTransparent = g.Transparency == "transparent"
	if len([]rune(e.Location)) > 500 {
		e.Location = string([]rune(e.Location)[:500])
	}
	if g.Start.Date != "" {
		d, err := domain.ParseDate(g.Start.Date)
		if err != nil {
			return nil, err
		}
		e.Date = d
		if end, err := domain.ParseDate(g.End.Date); err == nil {
			e.GoogleAllDayDays = int(end.Sub(d.Time) / (24 * time.Hour))
		}
	} else {
		start, err := time.Parse(time.RFC3339, g.Start.DateTime)
		if err != nil {
			return nil, err
		}
		end, err := time.Parse(time.RFC3339, g.End.DateTime)
		if err != nil {
			return nil, err
		}
		local := start.In(s.Location)
		e.Date = domain.Day(local)
		e.StartTime = &domain.TimeOfDay{Hour: local.Hour(), Minute: local.Minute()}
		e.StartSecond = local.Second()
		e.Kind = domain.EventMeeting
		e.Duration = end.Sub(start)
		e.GoogleDurationSeconds = int64(e.Duration / time.Second)
		if e.Duration < 5*time.Minute {
			e.Duration = 5 * time.Minute
		}
		if e.Duration > 24*time.Hour {
			e.Duration = 24 * time.Hour
		}
	}
	return e, e.Validate()
}
func (s *Service) toGoogle(e domain.Event) googleEvent {
	g := googleEvent{Summary: e.Title, Description: e.Description, Location: e.Location}
	if e.MeetingURL != "" && !strings.Contains(g.Description, e.MeetingURL) {
		g.Description = strings.TrimSpace(g.Description + "\n" + e.MeetingURL)
	}
	if e.Status == domain.StatusDone {
		g.Summary = "✓ " + g.Summary
	}
	if e.StartTime == nil {
		g.Start.Date = e.Date.String()
		days := e.GoogleAllDayDays
		if days < 1 {
			days = 1
		}
		g.End.Date = e.Date.AddDays(days).String()
	} else {
		start, end, _ := e.Interval(s.Location)
		if e.GoogleDurationSeconds > 0 && ((e.Duration == 24*time.Hour && e.GoogleDurationSeconds > 86400) || (e.Duration == 5*time.Minute && e.GoogleDurationSeconds < 300)) {
			end = start.Add(time.Duration(e.GoogleDurationSeconds) * time.Second)
		}
		g.Start = googleTime{DateTime: start.Format(time.RFC3339), TimeZone: s.Location.String()}
		g.End = googleTime{DateTime: end.Format(time.RFC3339), TimeZone: s.Location.String()}
	}
	return g
}
func (s *Service) pullGoogle(ctx context.Context, c *repository.CloudConnection, t *repository.OAuthToken) error {
	for reset := 0; reset < 2; reset++ {
		page := ""
		window := time.Now().In(s.Location)
		restart := false
		for n := 0; n < 1000; n++ {
			q := url.Values{"maxResults": {"250"}, "showDeleted": {"true"}, "singleEvents": {"true"}}
			if c.SyncToken != "" {
				q.Set("syncToken", c.SyncToken)
			} else {
				q.Set("timeMin", window.AddDate(-1, 0, 0).Format(time.RFC3339))
				q.Set("timeMax", window.AddDate(2, 0, 0).Format(time.RFC3339))
			}
			if page != "" {
				q.Set("pageToken", page)
			}
			var v struct {
				Items []googleEvent `json:"items"`
				Next  string        `json:"nextPageToken"`
				Sync  string        `json:"nextSyncToken"`
			}
			err := s.call(ctx, c, t, "GET", s.googleURL(c)+"?"+q.Encode(), nil, "", &v)
			if status(err, 410) && c.SyncToken != "" {
				c.SyncToken = ""
				if err = s.Store.SetCloudSyncToken(ctx, ""); err != nil {
					return err
				}
				restart = true
				break
			}
			if err != nil {
				return err
			}
			for _, g := range v.Items {
				if g.ID == "" {
					return errors.New("missing Google event id")
				}
				var e *domain.Event
				if g.Status != "cancelled" {
					e, err = s.fromGoogle(g)
					if err != nil {
						return err
					}
				}
				if err = s.Store.ImportGoogleEvent(ctx, c.CalendarID, repository.CloudLink{RemoteID: g.ID, ETag: g.ETag, Updated: g.Updated}, e, g.Status == "cancelled"); err != nil {
					return err
				}
			}
			if v.Next == "" {
				if v.Sync == "" {
					return errors.New("missing Google sync token")
				}
				c.SyncToken = v.Sync
				return s.Store.SetCloudSyncToken(ctx, v.Sync)
			}
			page = v.Next
		}
		if !restart {
			return errors.New("Google event pagination exceeded")
		}
	}
	return errors.New("Google full sync failed")
}
func (s *Service) pushGoogle(ctx context.Context, c *repository.CloudConnection, t *repository.OAuthToken) error {
	changes, err := s.Store.CloudChanges(ctx, "google")
	if err != nil {
		return err
	}
	for _, change := range changes {
		e, err := s.event(ctx, change.EventID)
		if err != nil {
			return err
		}
		link, err := s.Store.CloudLink(ctx, "google", c.CalendarID, change.EventID)
		if err != nil {
			return err
		}
		if e == nil || e.Date == nil {
			if link != nil {
				err = s.call(ctx, c, t, "DELETE", s.googleURL(c)+"/"+url.PathEscape(link.RemoteID)+"?sendUpdates=none", nil, "", nil)
				if err != nil && !status(err, 404) && !status(err, 410) {
					return err
				}
			}
		} else {
			g := s.toGoogle(*e)
			var saved googleEvent
			if link == nil {
				hash := sha256.Sum256([]byte(repository.UserID(ctx) + ":" + c.CalendarID + ":" + fmtID(e.ID)))
				g.ID = "gena" + hex.EncodeToString(hash[:16])
				// Persist deterministic identity before HTTP so a crash can never duplicate.
				link = &repository.CloudLink{EventID: e.ID, RemoteID: g.ID}
				if err = s.Store.SaveCloudLink(ctx, "google", c.CalendarID, *link); err != nil {
					return err
				}
			}
			var current googleEvent
			err = s.call(ctx, c, t, "GET", s.googleURL(c)+"/"+url.PathEscape(link.RemoteID), nil, "", &current)
			if status(err, 404) || status(err, 410) {
				g.ID = link.RemoteID
				err = s.call(ctx, c, t, "POST", s.googleURL(c)+"?sendUpdates=none", g, "", &saved)
				if status(err, 409) {
					return errors.New("Google event ID already exists; retry sync")
				}
			} else if err == nil {
				// Only managed fields are patched: attendees, conferenceData and provider
				// metadata survive. The fresh ETag prevents overwriting a concurrent edit.
				patch := map[string]any{"summary": g.Summary, "description": g.Description, "location": g.Location, "start": g.Start, "end": g.End}
				err = s.call(ctx, c, t, "PATCH", s.googleURL(c)+"/"+url.PathEscape(link.RemoteID)+"?sendUpdates=none", patch, current.ETag, &saved)
			}
			if err != nil {
				return err
			}
			link.ETag, link.Updated = saved.ETag, saved.Updated
			if err = s.Store.SaveCloudLink(ctx, "google", c.CalendarID, *link); err != nil {
				return err
			}
		}
		if err = s.Store.AckCloudChange(ctx, "google", change); err != nil {
			return err
		}
	}
	return nil
}
