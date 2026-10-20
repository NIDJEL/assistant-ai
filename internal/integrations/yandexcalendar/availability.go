package yandexcalendar

import (
	"context"
	"encoding/xml"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/repository"
	"github.com/NIDJEL/assistant-ai/internal/social"
)

func (s *Service) PlanningBusy(ctx context.Context, from, to time.Time) ([]social.BusySlot, error) {
	var result []social.BusySlot
	err := s.Store.WithCalendarLock(ctx, func() error {
		c, err := s.Store.CalendarCredentials(ctx)
		if err != nil {
			return err
		}
		if c == nil || c.Status == "disconnected" {
			return ErrAuthorization
		}
		token, err := s.decrypt(repository.UserID(ctx), c.Ciphertext)
		if err != nil {
			return err
		}
		if c.ExpiresAt != nil && time.Until(*c.ExpiresAt) < time.Minute {
			if err = s.refresh(ctx, c, &token); err != nil {
				return err
			}
		}
		remote, ok := s.Remote.(interface {
			PlanningBusy(context.Context, string, string, string, time.Time, time.Time, *time.Location) ([]social.BusySlot, error)
		})
		if !ok {
			return errors.New("calendar availability unsupported")
		}
		result, err = remote.PlanningBusy(ctx, c.Login, token.AccessToken, repository.UserID(ctx), from, to, s.Location)
		if errors.Is(err, ErrAuthorization) {
			if err = s.refresh(ctx, c, &token); err != nil {
				return err
			}
			result, err = remote.PlanningBusy(ctx, c.Login, token.AccessToken, repository.UserID(ctx), from, to, s.Location)
		}
		return err
	})
	return result, err
}
func (d *DAV) PlanningBusy(ctx context.Context, login, token, uid string, from, to time.Time, loc *time.Location) ([]social.BusySlot, error) {
	responses, err := d.properties(ctx, d.root(), login, token, `<d:current-user-principal/>`, "0")
	if err != nil {
		return nil, err
	}
	principal := ""
	for _, r := range responses {
		for _, p := range r.Props {
			if successful(p.Status) {
				principal = p.Property.Principal.Href
			}
		}
	}
	if principal == "" {
		return nil, errors.New("calendar principal missing")
	}
	responses, err = d.properties(ctx, principal, login, token, `<c:calendar-home-set/>`, "0")
	if err != nil {
		return nil, err
	}
	home := ""
	for _, r := range responses {
		for _, p := range r.Props {
			if successful(p.Status) {
				home = p.Property.Home.Href
			}
		}
	}
	if home == "" {
		return nil, errors.New("calendar home missing")
	}
	responses, err = d.properties(ctx, home, login, token, `<d:resourcetype/>`, "1")
	if err != nil {
		return nil, err
	}
	out := []social.BusySlot{}
	for _, r := range responses {
		calendar := false
		for _, p := range r.Props {
			calendar = calendar || successful(p.Status) && p.Property.Resource.Calendar != nil
		}
		if !calendar {
			continue
		}
		body := `<?xml version="1.0" encoding="utf-8"?><c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:calendar-data><c:comp name="VCALENDAR"><c:prop name="VERSION"/><c:comp name="VEVENT"><c:prop name="UID"/><c:prop name="DTSTART"/><c:prop name="DTEND"/><c:prop name="DURATION"/><c:prop name="STATUS"/><c:prop name="TRANSP"/></c:comp></c:comp><c:expand start="` + from.UTC().Format("20060102T150405Z") + `" end="` + to.UTC().Format("20060102T150405Z") + `"/></c:calendar-data></d:prop><c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="VEVENT"><c:time-range start="` + from.UTC().Format("20060102T150405Z") + `" end="` + to.UTC().Format("20060102T150405Z") + `"/></c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`
		raw, status, err := d.request(ctx, "REPORT", r.Href, login, token, body, "1")
		if err != nil {
			return nil, err
		}
		if status != 207 {
			return nil, &HTTPError{Operation: "availability REPORT", Status: status}
		}
		var report struct {
			Responses []struct {
				Props []struct {
					Status   string `xml:"status"`
					Property struct {
						Data string `xml:"calendar-data"`
					} `xml:"prop"`
				} `xml:"propstat"`
			} `xml:"response"`
		}
		if xml.Unmarshal(raw, &report) != nil {
			return nil, errors.New("invalid availability response")
		}
		for _, entry := range report.Responses {
			for _, p := range entry.Props {
				if !successful(p.Status) {
					return nil, errors.New("calendar availability incomplete")
				}
				slots, err := parseBusy(p.Property.Data, uid, loc)
				if err != nil {
					return nil, err
				}
				out = append(out, slots...)
			}
		}
	}
	return out, nil
}

// Parses only the restricted, server-expanded VEVENT time properties requested
// above. Unsupported or malformed values make availability unknown, never free.
func parseBusy(raw, uid string, loc *time.Location) ([]social.BusySlot, error) {
	raw = strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n ", ""), "\r\n\t", "")
	out := []social.BusySlot{}
	var start, end time.Time
	var eid int64
	skip := false
	inside := false
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if line == "BEGIN:VEVENT" {
			inside = true
			start = time.Time{}
			end = time.Time{}
			eid = 0
			skip = false
			continue
		}
		if line == "END:VEVENT" {
			if inside && !skip {
				if start.IsZero() || !end.After(start) {
					return nil, errors.New("calendar interval missing")
				}
				out = append(out, social.BusySlot{Slot: social.Slot{Start: start, End: end}, EventID: eid})
			}
			inside = false
			continue
		}
		if !inside {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(key, ";")
		switch name {
		case "UID":
			prefix := "rhythm-" + uid + "-"
			if strings.HasPrefix(value, prefix) && strings.HasSuffix(value, "@rhythm") {
				eid, _ = strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(value, prefix), "@rhythm"), 10, 64)
			}
		case "STATUS":
			skip = skip || value == "CANCELLED"
		case "TRANSP":
			skip = skip || value == "TRANSPARENT"
		case "DTSTART", "DTEND":
			zone := loc
			for _, param := range strings.Split(key, ";")[1:] {
				if strings.HasPrefix(param, "TZID=") {
					var err error
					zone, err = time.LoadLocation(strings.Trim(strings.TrimPrefix(param, "TZID="), `"`))
					if err != nil {
						return nil, err
					}
				}
			}
			var t time.Time
			var err error
			switch {
			case strings.HasSuffix(value, "Z"):
				t, err = time.Parse("20060102T150405Z", value)
			case len(value) == 8:
				t, err = time.ParseInLocation("20060102", value, zone)
			default:
				t, err = time.ParseInLocation("20060102T150405", value, zone)
			}
			if err != nil {
				return nil, err
			}
			if name == "DTSTART" {
				start = t
			} else {
				end = t
			}
		case "DURATION":
			duration, err := parseICalDuration(value)
			if err != nil {
				return nil, err
			}
			end = start.Add(duration)
		case "RRULE":
			return nil, errors.New("calendar did not expand recurrence")
		}
	}
	if inside {
		return nil, errors.New("truncated calendar event")
	}
	return out, nil
}
func parseICalDuration(v string) (time.Duration, error) {
	if strings.HasPrefix(v, "PT") {
		return time.ParseDuration(strings.ToLower(strings.TrimPrefix(v, "PT")))
	}
	if strings.HasPrefix(v, "P") && strings.HasSuffix(v, "D") {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(v, "P"), "D"))
		if err == nil && n > 0 && n <= 366 {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	return 0, errors.New("unsupported calendar duration")
}
