package api

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/config"
)

func TestProductionSPARoutesAndAPI404(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "web/dist/assets")
	if err := os.MkdirAll(dist, 0750); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "web/dist/index.html"), []byte("<html>React build</html>"), 0600)
	_ = os.WriteFile(filepath.Join(dist, "app.js"), []byte("console.log('build')"), 0600)
	_ = os.WriteFile(filepath.Join(root, "web/dist/notifications-sw.js"), []byte("self.addEventListener('notificationclick', () => {})"), 0600)
	a := API{Config: config.Config{Root: root}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	router := a.Router()
	for _, path := range []string{"/", "/calendar", "/tasks", "/meetings", "/friends", "/reminders", "/archive", "/integrations", "/settings"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "React build") || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("SPA route %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/assets/app.js", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "console.log") {
		t.Fatal("build assets not served")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/notifications-sw.js", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "notificationclick") || !strings.Contains(w.Header().Get("Content-Type"), "javascript") || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("notification worker not served as an updatable script")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/api/unknown", nil))
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"not_found"`) {
		t.Fatal("API 404 became an HTML fallback")
	}
}

func TestMissingBuildReturnsActionableError(t *testing.T) {
	a := API{Config: config.Config{Root: t.TempDir()}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	router := a.Router()
	for _, path := range []string{"/", "/auth", "/calendar"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 || !strings.Contains(w.Body.String(), `"frontend_unavailable"`) {
			t.Fatalf("missing build %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestBrowserErrorsKeepHTTPStatusAndAPIErrorsStayJSON(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "web", "dist", "errors")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"404", "503"} {
		if err := os.WriteFile(filepath.Join(dir, status+".html"), []byte("<html>Gena error "+status+"</html>"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := API{Config: config.Config{Root: root}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	for _, item := range []struct {
		path   string
		status int
	}{{"/missing-page", 404}, {"/errors/503", 503}} {
		request := httptest.NewRequest("GET", item.path, nil)
		request.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		a.Router().ServeHTTP(w, request)
		if w.Code != item.status || !strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), "Gena error") {
			t.Fatalf("browser error %s: %d %s", item.path, w.Code, w.Body.String())
		}
	}
	request := httptest.NewRequest("GET", "/api/missing", nil)
	request.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	a.Router().ServeHTTP(w, request)
	if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") || !strings.Contains(w.Body.String(), `"not_found"`) {
		t.Fatal("API errors must remain JSON")
	}
	request = httptest.NewRequest("HEAD", "/missing-page", nil)
	request.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	a.Router().ServeHTTP(w, request)
	if w.Code != 404 || w.Body.Len() != 0 {
		t.Fatal("HEAD error must not write a response body")
	}
}
