package contract

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/NIDJEL/assistant-ai/internal/api"
	"github.com/NIDJEL/assistant-ai/internal/calendar"
	"github.com/NIDJEL/assistant-ai/internal/clock"
	"github.com/NIDJEL/assistant-ai/internal/config"
)

func TestPostgresProfilePhotoPersistenceAndIsolation(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	_, alice := registerHTTP(t, router, "photo-alice@example.test")
	_, bob := registerHTTP(t, router, "photo-bob@example.test")
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	photo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData.Bytes())
	updated, _ := call(t, router, alice, "PATCH", "/api/auth/me", map[string]string{"avatar_url": photo}, 200)
	avatar := updated["user"].(map[string]any)["avatar_url"].(string)
	if !strings.HasPrefix(avatar, "data:image/jpeg;base64,") || updated["user"].(map[string]any)["name"] != "Пользователь" {
		t.Fatal("photo-only update changed the name or did not save photo")
	}
	call(t, router, alice, "PATCH", "/api/auth/me", map[string]string{"name": "Новое имя"}, 200)
	saved, _ := call(t, router, alice, "GET", "/api/auth/me", nil, 200)
	if saved["user"].(map[string]any)["avatar_url"] != avatar || saved["user"].(map[string]any)["name"] != "Новое имя" {
		t.Fatal("profile update was not persisted")
	}
	other, _ := call(t, router, bob, "GET", "/api/auth/me", nil, 200)
	if other["user"].(map[string]any)["avatar_url"] != "" {
		t.Fatal("photo leaked to another account")
	}
	call(t, router, nil, "PATCH", "/api/auth/me", map[string]string{"avatar_url": photo}, 401)
	call(t, router, alice, "PATCH", "/api/auth/me", map[string]string{"name": "Invalid overwrite", "avatar_url": "data:image/svg+xml;base64,PHN2Zz4="}, 422)
	unchanged, _ := call(t, router, alice, "GET", "/api/auth/me", nil, 200)
	if unchanged["user"].(map[string]any)["name"] != "Новое имя" {
		t.Fatal("failed avatar validation changed the name")
	}
	call(t, router, alice, "PATCH", "/api/auth/me", map[string]string{"name": " "}, 422)
	call(t, router, alice, "PATCH", "/api/auth/me", map[string]string{}, 422)
	removed, _ := call(t, router, alice, "PATCH", "/api/auth/me", map[string]string{"avatar_url": ""}, 200)
	if removed["user"].(map[string]any)["avatar_url"] != "" {
		t.Fatal("photo was not removed")
	}
}

func TestYandexDoubleSlashCallback(t *testing.T) {
	db, service, _, now := pgFixture(t)
	app := &api.API{Auth: service, Config: config.Config{Root: root(), PublicURL: "https://rhythm.test", YandexRedirectURI: "https://rhythm.test//api/auth/yandex/callback"}, Clock: clock.Func(func() time.Time { return now }), Calendar: &calendar.Service{Store: db}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	router := app.Router()
	start := httptest.NewRecorder()
	router.ServeHTTP(start, httptest.NewRequest("GET", "https://rhythm.test/api/auth/yandex/start", nil))
	redirect, _ := url.Parse(start.Header().Get("Location"))
	cookies := start.Result().Cookies()
	if start.Code != 303 || len(cookies) != 1 || cookies[0].Path != "//api/auth/yandex" {
		t.Fatal("callback cookie does not match registered path")
	}
	callback := httptest.NewRequest("GET", "https://rhythm.test//api/auth/yandex/callback?code=valid&state="+redirect.Query().Get("state"), nil)
	callback.AddCookie(cookies[0])
	finished := httptest.NewRecorder()
	router.ServeHTTP(finished, callback)
	if finished.Code != 303 || finished.Header().Get("Location") != "/onboarding" {
		t.Fatal("registered double-slash callback failed", finished.Code, finished.Header().Get("Location"))
	}
}
