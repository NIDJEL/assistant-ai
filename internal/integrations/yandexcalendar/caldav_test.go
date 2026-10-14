package yandexcalendar

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/NIDJEL/assistant-ai/internal/domain"
	"github.com/NIDJEL/assistant-ai/internal/repository"
)

func TestEnsureCalendarUsesCanonicalAddress(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		location, existing, unsafe bool
	}{
		{"location_header", true, false, false},
		{"rediscovery_without_location", false, false, false},
		{"existing_yandex_calendar", false, true, false},
		{"reject_external_location", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := tc.existing
			creations := 0
			canonical := "/calendars/user/events-123/"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "MKCALENDAR" {
					created = true
					creations++
					if tc.location {
						target := canonical
						if tc.unsafe {
							target = "https://attacker.test/calendar/"
						}
						w.Header().Set("Location", target)
					}
					w.WriteHeader(201)
					return
				}
				if r.Method != "PROPFIND" {
					w.WriteHeader(405)
					return
				}
				prop := `<d:resourcetype><d:collection/></d:resourcetype>`
				if r.URL.Path == "/" {
					prop = `<d:current-user-principal><d:href>/principals/user/</d:href></d:current-user-principal>`
				}
				if r.URL.Path == "/principals/user/" {
					prop = `<c:calendar-home-set><d:href>/calendars/user/</d:href></c:calendar-home-set>`
				}
				if r.URL.Path == canonical {
					prop = `<d:resourcetype><d:collection/><c:calendar/></d:resourcetype>`
				}
				w.WriteHeader(207)
				response := func(href, props string) string {
					return fmt.Sprintf(`<d:response><d:href>%s</d:href><d:propstat><d:prop>%s</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`, href, props)
				}
				body := response(r.URL.Path, prop)
				if r.URL.Path == "/calendars/user/" && created {
					body += response(canonical, `<d:displayname>Ритм</d:displayname><d:resourcetype><c:calendar/></d:resourcetype>`)
				}
				_, _ = fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">`+body+`</d:multistatus>`)
			}))
			defer server.Close()
			d := &DAV{BaseURL: server.URL + "/", Client: server.Client()}
			target, err := d.EnsureCalendar(context.Background(), "login", "secret", "user")
			if tc.unsafe {
				if err == nil {
					t.Fatal("unsafe Location accepted")
				}
				return
			}
			if err != nil || target != server.URL+canonical {
				t.Fatal("incorrect canonical address", target, err)
			}
			if tc.existing && creations != 0 || !tc.existing && creations != 1 {
				t.Fatal("calendar duplicated", creations)
			}
		})
	}
}

func TestCalendarURLsRejectCredentialExfiltration(t *testing.T) {
	d := &DAV{}
	for _, href := range []string{"https://attacker.test/", "//attacker.test/calendar/", "http://caldav.yandex.ru/", "https://user@caldav.yandex.ru/", "/calendar/?token=secret", "/calendar/#secret"} {
		if _, err := d.resolve(href); err == nil {
			t.Fatalf("unsafe href accepted: %s", href)
		}
	}
	if target, err := d.resolve("/calendars/test/"); err != nil || target != "https://caldav.yandex.ru/calendars/test/" {
		t.Fatal(target, err)
	}
	calls := 0
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	defer external.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, external.URL, 307) }))
	defer server.Close()
	d = &DAV{BaseURL: server.URL + "/", Client: server.Client()}
	if err := d.Put(context.Background(), server.URL+"/event.ics", "user", "secret", "ics"); err == nil || calls != 0 {
		t.Fatal("redirect forwarded credentials")
	}
}
func TestICalendarDatesEscapingCompletionAndUTF8Folding(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	day, _ := domain.ParseDate("2026-10-14")
	clock, _ := domain.ParseTime("23:30")
	e := domain.Event{ID: 7, Kind: domain.EventTask, Title: strings.Repeat("Проверка", 30) + ", тест;\nEND:VEVENT", Date: day, Duration: time.Hour, Status: domain.StatusOpen, Priority: domain.PriorityHigh, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	raw := eventCalendar(e, "user", loc)
	if !strings.Contains(raw, "DTSTART;VALUE=DATE:20261014\r\nDTEND;VALUE=DATE:20261015") || strings.Contains(raw, "\r\nEND:VEVENT\r\nEND:VEVENT") {
		t.Fatal("invalid all-day event or escaping")
	}
	for _, line := range strings.Split(raw, "\r\n") {
		if len(line) > 75 || !utf8.ValidString(line) {
			t.Fatal("invalid iCalendar line", len(line))
		}
	}
	e.StartTime = clock
	e.Status = domain.StatusDone
	raw = eventCalendar(e, "user", loc)
	if !strings.Contains(raw, "DTSTART:20261014T203000Z") || !strings.Contains(raw, "DTEND:20261014T213000Z") || !strings.Contains(raw, "SUMMARY:✓ ") || !strings.Contains(raw, "TRANSP:TRANSPARENT") {
		t.Fatal("incorrect timezone or completion")
	}
	e.Kind = domain.EventReminder
	e.Status = domain.StatusOpen
	if !strings.Contains(eventCalendar(e, "user", loc), "BEGIN:VALARM") {
		t.Fatal("reminder missing alarm")
	}
	disabled := false
	e.ReminderEnabled = &disabled
	if strings.Contains(eventCalendar(e, "user", loc), "BEGIN:VALARM") {
		t.Fatal("disabled alarm exported")
	}
}
func TestCredentialsEncryptedAndBoundToUser(t *testing.T) {
	s, err := New(nil, nil, nil, nil, "test-secret", time.UTC, nil)
	if err != nil {
		t.Fatal(err)
	}
	token := repository.OAuthToken{AccessToken: "secret-access", RefreshToken: "secret-refresh"}
	sealed, err := s.encrypt("alice", token)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), "secret-access") {
		t.Fatal("plaintext persisted")
	}
	if got, err := s.decrypt("alice", sealed); err != nil || got != token {
		t.Fatal("token roundtrip failed")
	}
	if _, err := s.decrypt("bob", sealed); err == nil {
		t.Fatal("token usable by another user")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := s.decrypt("alice", sealed); err == nil {
		t.Fatal("tampering accepted")
	}
}
