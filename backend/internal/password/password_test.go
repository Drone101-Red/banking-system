package password_test

import (
	"errors"
	"strings"
	"testing"

	"banking-system/internal/password"
)

func TestHash_Valid(t *testing.T) {
	hash, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "" {
		t.Fatal("hash vacío")
	}
	if hash == "TestPassword123!" {
		t.Fatal("hash debería diferir del plain")
	}
	if !strings.HasPrefix(hash, "$2a$12$") && !strings.HasPrefix(hash, "$2b$12$") {
		t.Fatalf("hash no parece bcrypt cost 12: %s", hash[:10])
	}
}

func TestHash_TooShort(t *testing.T) {
	_, err := password.Hash("short")
	if !errors.Is(err, password.ErrTooShort) {
		t.Fatalf("esperaba ErrTooShort, obtuve %v", err)
	}
}

func TestHash_TooLong(t *testing.T) {
	// 73 bytes
	long := strings.Repeat("a", 73)
	_, err := password.Hash(long)
	if !errors.Is(err, password.ErrTooLong) {
		t.Fatalf("esperaba ErrTooLong, obtuve %v", err)
	}
}

func TestHash_ExactlyMaxLength(t *testing.T) {
	// 72 bytes exactos — debe pasar
	exact := strings.Repeat("a", 72)
	if _, err := password.Hash(exact); err != nil {
		t.Fatalf("Hash con 72 bytes: %v", err)
	}
}

func TestCompare_Match(t *testing.T) {
	hash, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := password.Compare(hash, "TestPassword123!"); err != nil {
		t.Fatalf("Compare debería coincidir: %v", err)
	}
}

func TestCompare_Mismatch(t *testing.T) {
	hash, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	err = password.Compare(hash, "WrongPassword!")
	if !errors.Is(err, password.ErrMismatch) {
		t.Fatalf("esperaba ErrMismatch, obtuve %v", err)
	}
}

func TestHash_TwoHashesDifferButBothValid(t *testing.T) {
	// bcrypt incluye salt aleatorio, así que dos hashes del mismo
	// plain deben ser diferentes pero ambos válidos.
	h1, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("Hash 1: %v", err)
	}
	h2, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("Hash 2: %v", err)
	}

	if h1 == h2 {
		t.Fatal("dos hashes del mismo plain no deberían coincidir")
	}

	if err := password.Compare(h1, "TestPassword123!"); err != nil {
		t.Fatalf("Compare h1: %v", err)
	}
	if err := password.Compare(h2, "TestPassword123!"); err != nil {
		t.Fatalf("Compare h2: %v", err)
	}
}
