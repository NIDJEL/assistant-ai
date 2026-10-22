package auth

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	for _, name := range []string{"Анна", "Анна-Мария", "O'Connor", "D’Arcy", "Jose\u0301", "李 明", "𐐀𐐨", "Мария‑Луиза", strings.Repeat("Я", 100)} {
		if value, err := normalizeName(" " + name + " "); err != nil || value != name {
			t.Fatalf("valid international name rejected: %q, %v", name, err)
		}
	}
	for _, name := range []string{"", "  ", "Анна123", "Анна١", "Анна１２", "<script>", "Robert; DROP TABLE users", "Анна\nМария", "Анна\n", "Анна\r\n", "\tАнна", "Анна\x00", "Анна\u202e", "Анна\u200b", "\u0301Анна", "-Анна", "Анна-", "Анна--Мария", "Анна  Мария", "🙂", strings.Repeat("𐐀", 101), string([]byte{0xff})} {
		if _, err := normalizeName(name); err == nil {
			t.Fatalf("invalid name accepted: %q", name)
		}
	}
}

func TestInvalidNamesRejectedBeforePersistence(t *testing.T) {
	// A nil store also catches any accidental attempt to persist invalid input.
	s := &Service{}
	name := "Анна123"
	if _, err := s.Register(context.Background(), name, "anna@example.test", "test password 123", "test password 123"); err == nil {
		t.Fatal("registration accepted digits in a name")
	}
	if _, err := s.UpdateProfile(context.Background(), "user", &name, nil); err == nil {
		t.Fatal("profile accepted digits in a name")
	}
	if _, err := s.Update(context.Background(), "user", name, false); err == nil {
		t.Fatal("legacy update accepted digits in a name")
	}
}
