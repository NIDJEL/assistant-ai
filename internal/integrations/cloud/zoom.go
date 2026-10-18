package cloud

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func fmtID(id int64) string { return strconv.FormatInt(id, 10) }

type zoomMeeting struct {
	ID        int64  `json:"id"`
	Topic     string `json:"topic"`
	Agenda    string `json:"agenda"`
	StartTime string `json:"start_time"`
	JoinURL   string `json:"join_url"`
}

func (s *Service) zoomMarker(ctx context.Context, id int64) string {
	return "gena:" + repository.UserID(ctx) + ":" + fmtID(id)
}
func (s *Service) recoverZoom(ctx context.Context, c *repository.CloudConnection, t *repository.OAuthToken, e domain.Event) (*zoomMeeting, error) {
	page := ""
	for i := 0; i < 100; i++ {
		var result struct {
			Meetings []zoomMeeting `json:"meetings"`
			Next     string        `json:"next_page_token"`
		}
		q := url.Values{"type": {"scheduled"}, "page_size": {"100"}}
		if page != "" {
			q.Set("next_page_token", page)
		}
		if err := s.call(ctx, c, t, "GET", s.ZoomBase+"/users/me/meetings?"+q.Encode(), nil, "", &result); err != nil {
			return nil, err
		}
		for _, m := range result.Meetings {
			if m.Topic != e.Title {
				continue
			}
			var full zoomMeeting
			if err := s.call(ctx, c, t, "GET", s.ZoomBase+"/meetings/"+fmtID(m.ID), nil, "", &full); err != nil {
				return nil, err
			}
			if strings.Contains(full.Agenda, s.zoomMarker(ctx, e.ID)) {
				return &full, nil
			}
		}
		if result.Next == "" {
			return nil, nil
		}
		page = result.Next
	}
	return nil, errors.New("Zoom reconciliation exceeded")
}
func (s *Service) pushZoom(ctx context.Context, c *repository.CloudConnection, t *repository.OAuthToken) error {
	changes, err := s.Store.CloudChanges(ctx, "zoom")
	if err != nil {
		return err
	}
	for _, change := range changes {
		e, err := s.event(ctx, change.EventID)
		if err != nil {
			return err
		}
		link, err := s.Store.CloudLink(ctx, "zoom", "", change.EventID)
		if err != nil {
			return err
		}
		if e == nil {
			if link != nil && !strings.HasPrefix(link.RemoteID, "pending:") {
				err = s.call(ctx, c, t, "DELETE", s.ZoomBase+"/meetings/"+url.PathEscape(link.RemoteID), nil, "", nil)
				if err != nil && !status(err, 404) {
					return err
				}
			}
		} else if e.ConferenceProvider == "zoom" || link != nil {
			start, _, valid := e.Interval(s.Location)
			if !valid {
				return domain.Validation("Для Zoom нужны дата и время встречи")
			}
			body := map[string]any{"topic": e.Title, "type": 2, "start_time": start.UTC().Format(time.RFC3339), "timezone": s.Location.String(), "duration": int(e.Duration / time.Minute), "agenda": s.zoomMarker(ctx, e.ID)}
			var meeting zoomMeeting
			if link != nil && strings.HasPrefix(link.RemoteID, "pending:") {
				found, err := s.recoverZoom(ctx, c, t, *e)
				if err != nil {
					return err
				}
				if found == nil {
					// Zoom has no idempotency key for this endpoint. Never automatically
					// repeat an ambiguous POST: a timeout may already have created a meeting.
					return &domain.AppError{Code: "zoom_creation_uncertain", Message: "Zoom не подтвердил создание. Проверьте список встреч Zoom перед повторным созданием; автоматический повтор остановлен, чтобы не создать дубль.", Status: 409}
				}
				meeting = *found
				link.RemoteID = fmtID(meeting.ID)
				if err = s.Store.SaveCloudLink(ctx, "zoom", "", *link); err != nil {
					return err
				}
			}
			if link == nil {
				link = &repository.CloudLink{EventID: e.ID, RemoteID: "pending:" + fmtID(e.ID)}
				if err = s.Store.SaveCloudLink(ctx, "zoom", "", *link); err != nil {
					return err
				}
				if err = s.call(ctx, c, t, "POST", s.ZoomBase+"/users/me/meetings", body, "", &meeting); err != nil {
					return err
				}
				if meeting.ID <= 0 || meeting.JoinURL == "" {
					return errors.New("invalid Zoom meeting response")
				}
				link.RemoteID = fmtID(meeting.ID)
				if err = s.Store.SaveCloudLink(ctx, "zoom", "", *link); err != nil {
					return err
				}
			} else {
				if err = s.call(ctx, c, t, "PATCH", s.ZoomBase+"/meetings/"+url.PathEscape(link.RemoteID), body, "", nil); err != nil {
					return err
				}
				if err = s.call(ctx, c, t, "GET", s.ZoomBase+"/meetings/"+url.PathEscape(link.RemoteID), nil, "", &meeting); err != nil {
					return err
				}
			}
			u, err := url.Parse(meeting.JoinURL)
			if err != nil || u.Scheme != "https" || u.User != nil || !(u.Hostname() == "zoom.us" || strings.HasSuffix(u.Hostname(), ".zoom.us") || u.Hostname() == "zoomgov.com" || strings.HasSuffix(u.Hostname(), ".zoomgov.com")) {
				return errors.New("invalid Zoom join URL")
			}
			if e.MeetingURL != meeting.JoinURL {
				if err = s.Store.Write(ctx, func(tx repository.Tx) error {
					current, err := tx.Get(ctx, e.ID)
					if err != nil || current == nil {
						return err
					}
					current.MeetingURL = meeting.JoinURL
					current.ConferenceProvider = "zoom"
					_, err = tx.Update(ctx, *current, time.Now())
					return err
				}); err != nil {
					return err
				}
			}
		}
		if err = s.Store.AckCloudChange(ctx, "zoom", change); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateZoom(ctx context.Context, id int64) (*domain.Event, error) {
	if err := s.Ready(ctx, "zoom"); err != nil {
		return nil, err
	}
	if err := s.Store.Write(ctx, func(tx repository.Tx) error {
		e, err := tx.Get(ctx, id)
		if err != nil {
			return err
		}
		if e == nil {
			return &domain.AppError{Code: "not_found", Message: "Встреча не найдена", Status: 404}
		}
		if e.Kind != domain.EventMeeting {
			return domain.Validation("Выберите встречу")
		}
		e.ConferenceProvider = "zoom"
		_, err = tx.Update(ctx, *e, time.Now())
		return err
	}); err != nil {
		return nil, err
	}
	return s.SyncEvent(ctx, id, false, true)
}
