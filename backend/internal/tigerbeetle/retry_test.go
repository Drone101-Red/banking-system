package tigerbeetle

import (
	"errors"
	"testing"
	"time"
)

func TestRetry_SucceedsAfterFailures(t *testing.T) {
	attempts := 0

	err := retry(5, 1*time.Millisecond, func() error {
		attempts++

		if attempts < 3 {
			return errors.New("TigerBeetle no disponible")
		}

		return nil
	})

	if err != nil {
		t.Fatalf("se esperaba éxito, se obtuvo error: %v", err)
	}

	if attempts != 3 {
		t.Fatalf("se esperaban 3 intentos, se realizaron %d", attempts)
	}
}

func TestRetry_ReturnsErrorAfterMaxAttempts(t *testing.T) {
	attempts := 0
	expectedErr := errors.New("TigerBeetle no disponible")

	err := retry(5, 1*time.Millisecond, func() error {
		attempts++
		return expectedErr
	})

	if err == nil {
		t.Fatal("se esperaba un error, se obtuvo nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf("se esperaba el error original, se obtuvo: %v", err)
	}

	if attempts != 5 {
		t.Fatalf("se esperaban 5 intentos, se realizaron %d", attempts)
	}
}
