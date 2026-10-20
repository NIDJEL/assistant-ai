package api

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/auth"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
)

func csvRequest(t *testing.T, name, content string, extra bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, content)
	if extra {
		_ = writer.WriteField("unexpected", "value")
	}
	_ = writer.Close()
	r := httptest.NewRequest("POST", "/api/import-csv", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Origin", "https://gena.test")
	return r
}

func TestCSVUploadRejectsInvalidFilesBeforeWriting(t *testing.T) {
	valid := "id,date,time,text\n1,2026-12-01,10:00,Отчёт\n"
	for _, tc := range []struct {
		name, filename, body string
		extra                bool
		status               int
	}{
		{"extension", "events.txt", valid, false, 415},
		{"empty", "events.csv", "", false, 400},
		{"oversized", "events.csv", strings.Repeat("x", calendar.CSVMaxBytes+1), false, 413},
		{"extra part", "events.csv", valid, true, 400},
		{"encoding", "events.csv", valid + "\xff", false, 422},
		{"missing headers", "events.csv", "date,text\n2026-12-01,Отчёт\n", false, 422},
		{"duplicate headers", "events.csv", "id,date,time,text,text\n1,2026-12-01,10:00,Отчёт,Дубль\n", false, 422},
		{"no records", "events.csv", "id,date,time,text\n", false, 422},
		{"invalid late row", "events.csv", valid + "2,2026-02-30,10:00,Ошибка\n", false, 422},
		{"too many rows", "events.csv", "id,date,time,text\n" + func() string {
			var rows strings.Builder
			for i := 1; i <= calendar.CSVMaxRows+1; i++ {
				fmt.Fprintf(&rows, "%d,2026-12-01,10:00,Отчёт\n", i)
			}
			return rows.String()
		}(), false, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A nil store makes an accidental write in any invalid case fail the test.
			a := &API{Calendar: &calendar.Service{}, Config: config.Config{PublicURL: "https://gena.test"},
				Clock: clock.Func(func() time.Time { return time.Now() }), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
			w := httptest.NewRecorder()
			a.csrf(http.HandlerFunc(a.importCSV)).ServeHTTP(w, csvRequest(t, tc.filename, tc.body, tc.extra))
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}

func TestCSVUploadRequiresAuthAndMatchingOrigin(t *testing.T) {
	a := &API{Auth: &auth.Service{}, Config: config.Config{PublicURL: "https://gena.test"}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w := httptest.NewRecorder()
	a.Router().ServeHTTP(w, csvRequest(t, "events.csv", "id,date,time,text\n", false))
	if w.Code != 401 {
		t.Fatal("anonymous upload accepted", w.Code)
	}
	for _, origin := range []string{"", "https://other.test"} {
		r := csvRequest(t, "events.csv", "id,date,time,text\n", false)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.csrf(http.HandlerFunc(a.importCSV)).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("untrusted origin accepted", w.Code)
		}
	}
}
