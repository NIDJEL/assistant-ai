package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NIDJEL/assistant-ai/internal/api"
)

func TestRemovedProviderRoutesAndOpenAPI(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	for _, suffix := range []string{"start", "callback"} {
		call(t, router, nil, "GET", "/api/auth/vk/"+suffix, nil, 404)
	}
	call(t, router, nil, "GET", "/api/auth/me", nil, 401)
	_, cookie := registerHTTP(t, router, "identity@example.test")
	call(t, router, cookie, "GET", "/api/auth/me", nil, 200)
	raw, _ := json.Marshal(api.OpenAPI())
	if strings.Contains(strings.ToLower(string(raw)), "/vk/") || !strings.Contains(string(raw), "Ассистент Гена") {
		t.Fatal(string(raw))
	}
}

func TestProviderRemovalMigrationKeepsAccountsAndEvents(t *testing.T) {
	db, _, router, _ := pgFixture(t)
	uid, cookie := registerHTTP(t, router, "retired@example.test")
	call(t, router, cookie, "POST", "/api/events", map[string]string{"kind": "task", "title": "Пользовательский Ритм"}, 201)
	// Reconstruct an already-applied previous schema, then run only the new migration.
	_, err := db.DB.Exec(`DELETE FROM schema_migrations WHERE name='003_remove_vk.sql';
ALTER TABLE auth_identities DROP CONSTRAINT auth_identities_provider_check;
ALTER TABLE auth_identities ADD CONSTRAINT auth_identities_provider_check CHECK(provider IN ('email','yandex','vk'));`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`INSERT INTO auth_identities(id,user_id,provider,provider_subject) VALUES(gen_random_uuid(),$1,'vk','retired');
`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`INSERT INTO oauth_states(id,provider,browser_hash,verifier,expires_at) VALUES('retired','vk','browser','verifier',now()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"auth_identities", "oauth_states"} {
		var n int
		if err = db.DB.QueryRow("SELECT count(*) FROM " + table + " WHERE provider='vk'").Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
	var title string
	if err = db.DB.QueryRow("SELECT title FROM events WHERE user_id=$1", uid).Scan(&title); err != nil || title != "Пользовательский Ритм" {
		t.Fatal("user content modified", title, err)
	}
	call(t, router, cookie, "GET", "/api/auth/me", nil, 200)
}

func TestCurrentBrandMetadataAndConfiguration(t *testing.T) {
	for _, file := range []string{"web/index.html"} {
		raw, err := os.ReadFile(filepath.Join(root(), file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "Ассистент Гена") || strings.Contains(string(raw), "Ритм") {
			t.Fatal(file, string(raw))
		}
	}
	for _, file := range []string{".env.example", "compose.yaml", "deploy/compose.production.yaml", "internal/config/config.go", "cmd/server/main.go"} {
		raw, err := os.ReadFile(filepath.Join(root(), file))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "VK_") || strings.Contains(string(raw), "integrations/vk") {
			t.Fatal("retired provider config remains", file)
		}
	}
}
