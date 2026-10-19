package cloud

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
)

// PlanningBusy requests only time/transparency/IDs; personal event text never
// enters the shared-planning response or cache. Recurrences expand at Google.
func (s *Service) PlanningBusy(ctx context.Context, from, to time.Time) ([]social.BusySlot, error) {
	out := []social.BusySlot{}
	err := s.authorized(ctx, "google", func(c *repository.CloudConnection, t *repository.OAuthToken) error {
		calendars := []Calendar{}
		page := ""
		for n := 0; n < 20; n++ {
			var list struct {
				Items []Calendar `json:"items"`
				Next  string     `json:"nextPageToken"`
			}
			q := url.Values{"maxResults": {"250"}, "fields": {"items(id,accessRole,timeZone),nextPageToken"}}
			if page != "" {
				q.Set("pageToken", page)
			}
			if err := s.call(ctx, c, t, "GET", s.GoogleBase+"/users/me/calendarList?"+q.Encode(), nil, "", &list); err != nil {
				return err
			}
			calendars = append(calendars, list.Items...)
			page = list.Next
			if page == "" {
				break
			}
		}
		if page != "" {
			return errors.New("calendar pagination exceeded")
		}
		for _, cal := range calendars {
			zone := s.Location
			if cal.TimeZone != "" {
				var err error
				zone, err = time.LoadLocation(cal.TimeZone)
				if err != nil {
					return err
				}
			}
			page = ""
			for n := 0; n < 100; n++ {
				var list struct {
					Items []struct {
						ID           string     `json:"id"`
						Start        googleTime `json:"start"`
						End          googleTime `json:"end"`
						Status       string     `json:"status"`
						Transparency string     `json:"transparency"`
					} `json:"items"`
					Next string `json:"nextPageToken"`
				}
				q := url.Values{"timeMin": {from.Format(time.RFC3339)}, "timeMax": {to.Format(time.RFC3339)}, "singleEvents": {"true"}, "maxResults": {"2500"}, "fields": {"items(id,start,end,status,transparency),nextPageToken"}}
				if page != "" {
					q.Set("pageToken", page)
				}
				if err := s.call(ctx, c, t, "GET", s.GoogleBase+"/calendars/"+url.PathEscape(cal.ID)+"/events?"+q.Encode(), nil, "", &list); err != nil {
					return err
				}
				for _, e := range list.Items {
					if e.Status == "cancelled" || e.Transparency == "transparent" {
						continue
					}
					var start, end time.Time
					var err error
					if e.Start.Date != "" {
						start, err = time.ParseInLocation("2006-01-02", e.Start.Date, zone)
						if err == nil {
							end, err = time.ParseInLocation("2006-01-02", e.End.Date, zone)
						}
					} else {
						start, err = time.Parse(time.RFC3339, e.Start.DateTime)
						if err == nil {
							end, err = time.Parse(time.RFC3339, e.End.DateTime)
						}
					}
					if err != nil {
						return err
					}
					eid := int64(0)
					if store, ok := s.Store.(interface {
						PlanningEvent(context.Context, string, string, string) (int64, error)
					}); ok {
						eid, err = store.PlanningEvent(ctx, "google", cal.ID, e.ID)
						if err != nil {
							return err
						}
					}
					out = append(out, social.BusySlot{Slot: social.Slot{Start: start, End: end}, EventID: eid})
				}
				page = list.Next
				if page == "" {
					break
				}
			}
			if page != "" {
				return errors.New("event pagination exceeded")
			}
		}
		return nil
	})
	return out, err
}
