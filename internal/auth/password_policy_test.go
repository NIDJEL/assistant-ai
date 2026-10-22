package auth

import (
	"context"
	"strings"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	for _, value := range []string{"River!coast7", "test password 12345", "ёжик7СинийЛес!", "my new Gena password", "Лампа светит возле окна", "  River!coast7  ", "🪐Rivers7🌊moon"} {
		if err := validatePassword(value); err != nil {
			t.Fatalf("valid password rejected: %q: %v", value, err)
		}
	}
	for _, value := range []string{"", strings.Repeat(" ", 10), strings.Repeat("\u00a0", 20), "abcdefgh9" + strings.Repeat(" ", 50), strings.Repeat("A", 20), "abcabcabcabc", "1234567890", "abcdefghij", "Qwertyuiop", "password12345", "Password123!", "пароль12345", "welcome123456", "5816094732", "riverforest", "word word word word", "Valid!pass7\u200b", "Valid!pass7\n", strings.Repeat("x", 1025)} {
		if err := validatePassword(value); err == nil {
			t.Fatalf("weak password accepted: %q", value)
		}
		service := &Service{}
		if _, err := service.Register(context.Background(), "Анна", "anna@example.test", value, value); err == nil {
			t.Fatal("registration bypassed password policy")
		}
		if _, err := service.EnableEmailLogin(context.Background(), "user", value, value); err == nil {
			t.Fatal("email enrollment bypassed password policy")
		}
	}
}

func TestPasswordUTF8LimitAndExactValue(t *testing.T) {
	value := "Ab!7" + strings.Repeat("Я", 510)
	if err := validatePassword(value); err != nil {
		t.Fatal("1024 UTF-8 bytes rejected", err)
	}
	if err := validatePassword(value + "Я"); err == nil {
		t.Fatal("oversized UTF-8 password accepted")
	}
	hash, err := HashPassword("  River!coast7  ")
	if err != nil || !VerifyPassword("  River!coast7  ", hash) || VerifyPassword("River!coast7", hash) {
		t.Fatal("significant spaces were changed", err)
	}
}
