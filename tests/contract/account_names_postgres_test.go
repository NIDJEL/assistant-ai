package contract

import (
	"strings"
	"testing"
)

func TestPostgresAccountNameValidation(t *testing.T) {
	_, _, router, _ := pgFixture(t)
	password := "test password 12345"
	for _, name := range []string{"Анна123", "Анна١", "<script>alert</script>", "Анна\nМария", "Анна\u202e", "", strings.Repeat("Я", 101)} {
		call(t, router, nil, "POST", "/api/auth/register", withLegal(map[string]string{"name": name, "email": "names@example.test", "password": password, "password_confirmation": password}), 422)
	}
	// Reusing that email proves no rejected registration reserved an account.
	_, cookie := registerHTTP(t, router, "names@example.test")
	for _, name := range []string{"Анна123", "Анна\u200b", "Анна\x00", "-Анна", strings.Repeat("Я", 101)} {
		call(t, router, cookie, "PATCH", "/api/auth/me", map[string]string{"name": name}, 422)
		data, _ := call(t, router, cookie, "GET", "/api/auth/me", nil, 200)
		if data["user"].(map[string]any)["name"] != "Пользователь" {
			t.Fatal("rejected update changed the saved name")
		}
	}
	for _, name := range []string{"Анна-Мария", "O’Connor", "Jose\u0301", "李 明"} {
		data, _ := call(t, router, cookie, "PATCH", "/api/auth/me", map[string]string{"name": " " + name + " "}, 200)
		if data["user"].(map[string]any)["name"] != name {
			t.Fatal("international name was altered")
		}
	}
	call(t, router, cookie, "POST", "/api/auth/onboarding", map[string]any{}, 200)
}
