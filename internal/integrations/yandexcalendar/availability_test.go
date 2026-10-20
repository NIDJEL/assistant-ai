package yandexcalendar

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPlanningDAVReadsAllCalendarsWithRestrictedProperties(t *testing.T) {
	reports := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != "login" || token != "test-token" {
			t.Error("missing scoped authorization")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(207)
		var prop string
		switch r.URL.Path {
		case "/":
			prop = `<d:current-user-principal><d:href>/principal/</d:href></d:current-user-principal>`
		case "/principal/":
			prop = `<c:calendar-home-set><d:href>/calendars/</d:href></c:calendar-home-set>`
		case "/calendars/":
			fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>/one/</d:href><d:propstat><d:prop><d:resourcetype><c:calendar/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response><d:response><d:href>/two/</d:href><d:propstat><d:prop><d:resourcetype><c:calendar/></d:resourcetype></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`)
			return
		case "/one/", "/two/":
			if r.Method != "REPORT" {
				t.Error(r.Method)
			}
			raw, _ := io.ReadAll(r.Body)
			for _, private := range []string{"SUMMARY", "DESCRIPTION", "ATTENDEE"} {
				if strings.Contains(string(raw), private) {
					t.Error("requested private field", private)
				}
			}
			if !strings.Contains(string(raw), "c:expand") {
				t.Error("recurrence not expanded")
			}
			reports++
			prop = `<c:calendar-data>BEGIN:VCALENDAR&#13;&#10;BEGIN:VEVENT&#13;&#10;UID:private-uid&#13;&#10;DTSTART:20261021T120000Z&#13;&#10;DTEND:20261021T130000Z&#13;&#10;END:VEVENT&#13;&#10;END:VCALENDAR</c:calendar-data>`
		default:
			t.Error("unexpected target", r.URL.Path)
		}
		fmt.Fprintf(w, `<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:response><d:href>%s</d:href><d:propstat><d:prop>%s</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, r.URL.Path, prop)
	}))
	defer server.Close()
	dav := DAV{BaseURL: server.URL + "/", Client: server.Client()}
	from := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	slots, err := dav.PlanningBusy(t.Context(), "login", "test-token", "user", from, from.AddDate(0, 0, 2), time.UTC)
	if err != nil || reports != 2 || len(slots) != 2 {
		t.Fatal(slots, reports, err)
	}
}
func TestPlanningBusyParserTimezonesTransparencyAndMalformedData(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	raw := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:rhythm-user-42@rhythm\r\nDTSTART;TZID=Europe/Moscow:20261021T150000\r\nDURATION:PT30M\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:transparent\r\nDTSTART:20261021T160000Z\r\nDTEND:20261021T170000Z\r\nTRANSP:TRANSPARENT\r\nEND:VEVENT\r\nEND:VCALENDAR"
	slots, err := parseBusy(raw, "user", loc)
	if err != nil || len(slots) != 1 || slots[0].EventID != 42 || slots[0].Start.UTC().Hour() != 12 || slots[0].End.Sub(slots[0].Start) != 30*time.Minute {
		t.Fatal(slots, err)
	}
	if _, err = parseBusy("BEGIN:VEVENT\nDTSTART:bad\nEND:VEVENT", "user", loc); err == nil {
		t.Fatal("malformed interval treated as free")
	}
}
