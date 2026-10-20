package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func uploadCSV(t *testing.T, router http.Handler, cookie *http.Cookie, filename string, data []byte, status int) map[string]any {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write(data)
	_ = writer.Close()
	r := httptest.NewRequest("POST", "/api/import-csv", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Origin", "https://rhythm.test")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("CSV status=%d expected=%d body=%s", w.Code, status, w.Body.String())
	}
	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPostgresCSVUploadIsAtomicPrivateAndIndependentOfFilename(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	_, alice := registerHTTP(t, router, "csv-alice@example.test")
	_, bob := registerHTTP(t, router, "csv-bob@example.test")
	body := []byte("id,date,time,text\n1,2026-12-01,10:00,Подготовить отчёт\n2,2026-12-02,15:00,Встреча с командой\n")
	first := uploadCSV(t, router, alice, "events.csv", body, 200)
	if first["imported"] != float64(2) || first["total_rows"] != float64(2) {
		t.Fatal(first)
	}
	repeated := uploadCSV(t, router, alice, "renamed.CSV", append([]byte{0xef, 0xbb, 0xbf}, bytes.ReplaceAll(body, []byte("\n"), []byte("\r\n"))...), 200)
	if repeated["imported"] != float64(0) || repeated["skipped"] != float64(2) {
		t.Fatal("renaming/BOM/newlines duplicated records", repeated)
	}
	other, _ := call(t, router, bob, "GET", "/api/events", nil, 200)
	if other["total"] != float64(0) {
		t.Fatal("import escaped account", other)
	}
	if own := uploadCSV(t, router, bob, "events.csv", body, 200); own["imported"] != float64(2) {
		t.Fatal("other account cannot import independently", own)
	}
	changed := []byte("id,date,time,text\n1,2026-12-03,11:00,Совсем другой файл\n")
	if imported := uploadCSV(t, router, alice, "events.csv", changed, 200); imported["imported"] != float64(1) {
		t.Fatal("same name hid a different file", imported)
	}
	invalid := append([]byte("id,date,time,text\n3,2026-12-05,10:00,Валидная строка\n"), []byte("4,2026-02-31,11:00,Ошибка\n")...)
	uploadCSV(t, router, alice, "broken.csv", invalid, 422)
	listed, _ := call(t, router, alice, "GET", "/api/events", nil, 200)
	if listed["total"] != float64(3) {
		t.Fatal("invalid batch was partially written", listed)
	}
	for _, value := range listed["events"].([]any) {
		event := value.(map[string]any)
		if event["collaborative"] == true {
			t.Fatal("CSV created invitations", event)
		}
	}
}

func TestPostgresCSVUploadAndDemoShareExistingKeys(t *testing.T) {
	file, err := os.Open(filepath.Join(root(), "data/voice_assistant_events_1000.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, demoFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "demo first", false: "upload first"}[demoFirst], func(t *testing.T) {
			_, _, router, _ := pgFixture(t)
			_, account := registerHTTP(t, router, "csv-demo@example.test")
			var first, second map[string]any
			if demoFirst {
				first, _ = call(t, router, account, "POST", "/api/import-demo", map[string]any{}, 200)
				second = uploadCSV(t, router, account, "voice_assistant_events_1000_77e927b5d2.csv", body, 200)
			} else {
				first = uploadCSV(t, router, account, "renamed.csv", body, 200)
				second, _ = call(t, router, account, "POST", "/api/import-demo", map[string]any{}, 200)
			}
			if first["imported"] != float64(1000) || second["imported"] != float64(0) || second["skipped"] != float64(1000) {
				t.Fatal(first, second)
			}
		})
	}
}
