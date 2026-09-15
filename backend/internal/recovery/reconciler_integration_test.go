//go:build integration

package recovery_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/password"
	"banking-system/internal/recovery"
	"banking-system/internal/tigerbeetle"
)

// testEnv encapsula las dependencias reales para tests de integración.
type testEnv struct {
	pg         *db.PostgresStore
	tb         *tigerbeetle.Client
	reconciler *recovery.Reconciler
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pgDSN := os.Getenv("POSTGRES_TEST_DSN")
	if pgDSN == "" {
		pgDSN = "host=172.30.0.2 port=5432 user=banking_user password=secret123 dbname=banking sslmode=disable"
	}
	tbAddress := os.Getenv("TB_TEST_ADDRESS")
	if tbAddress == "" {
		tbAddress = "172.30.0.10:3000"
	}

	pg, err := db.NewPostgresStore(pgDSN)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })

	tbClient, err := tigerbeetle.New(tbAddress, "0")
	if err != nil {
		t.Fatalf("tigerbeetle: %v", err)
	}
	t.Cleanup(func() { tbClient.Close() })

	return &testEnv{
		pg:         pg,
		tb:         tbClient,
		reconciler: recovery.NewReconciler(pg, tbClient),
	}
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@test.local", prefix, time.Now().UnixNano())
}

func randomTBID(t *testing.T) (tb.Uint128, []byte) {
	t.Helper()

	for {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatalf("rand.Read: %v", err)
		}
		id := tb.BytesToUint128(b)
		if id == tb.ToUint128(0) || id == tb.ToUint128(1) {
			continue
		}
		return id, b[:]
	}
}

// createPendingUser inserta un usuario PENDING directamente, sin
// crear su cuenta en TigerBeetle. Es exactamente el escenario que
// el reconciliador debe recuperar.
func createPendingUser(t *testing.T, env *testEnv) (*models.User, tb.Uint128) {
	t.Helper()
	ctx := context.Background()

	tbID, tbIDBytes := randomTBID(t)
	hash, err := password.Hash("TestPassword123!")
	if err != nil {
		t.Fatalf("password.Hash: %v", err)
	}

	u := &models.User{
		Email:        uniqueEmail("rec"),
		PasswordHash: hash,
		FullName:     "Recovery Test",
		TBAccountID:  tbIDBytes,
	}
	if err := env.pg.CreateUserPending(ctx, u); err != nil {
		t.Fatalf("CreateUserPending: %v", err)
	}
	return u, tbID
}

// --- Tests ---

func TestReconcileOnce_PendingWithoutTB(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user, tbID := createPendingUser(t, env)

	// Verificar que TB NO tiene la cuenta (es el escenario PENDING real).
	exists, err := env.tb.AccountExists(tbID)
	if err != nil {
		t.Fatalf("AccountExists antes: %v", err)
	}
	if exists {
		t.Fatal("TB no debería tener la cuenta al inicio")
	}

	// Reconciliar.
	processed, failed, err := env.reconciler.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}
	if failed != 0 {
		t.Fatalf("esperaba 0 fallidos, obtuve %d", failed)
	}
	if processed < 1 {
		t.Fatalf("esperaba al menos 1 procesado, obtuve %d", processed)
	}

	// Verificar que el usuario pasó a ACTIVE.
	fetched, err := env.pg.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if fetched.Status != models.StatusActive {
		t.Fatalf("status = %s, esperado ACTIVE", fetched.Status)
	}

	// Verificar que la cuenta TB ahora existe.
	exists, err = env.tb.AccountExists(tbID)
	if err != nil {
		t.Fatalf("AccountExists después: %v", err)
	}
	if !exists {
		t.Fatal("TB debería tener la cuenta después del reconciliador")
	}
}

func TestReconcileOnce_PendingWithTB(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user, tbID := createPendingUser(t, env)

	// Crear la cuenta en TB manualmente (simula una ejecución previa
	// del reconciliador que creó la cuenta pero no llegó a actualizar PG).
	if err := env.tb.CreateAccount(tbID, models.CodeSavings); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	// Reconciliar.
	_, failed, err := env.reconciler.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}
	if failed != 0 {
		t.Fatalf("esperaba 0 fallidos, obtuve %d", failed)
	}

	// El usuario debe haber pasado a ACTIVE.
	fetched, err := env.pg.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if fetched.Status != models.StatusActive {
		t.Fatalf("status = %s, esperado ACTIVE", fetched.Status)
	}
}

func TestReconcileOnce_Idempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	user, tbID := createPendingUser(t, env)

	// Primera ejecución.
	if _, _, err := env.reconciler.ReconcileOnce(ctx); err != nil {
		t.Fatalf("primera: %v", err)
	}

	// Segunda ejecución: el usuario ya está ACTIVE, no debe procesarlo.
	processed, failed, err := env.reconciler.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	// No hay PENDINGs nuevos, pero puede haber otros de tests previos.
	// Lo importante: no debe fallar.
	if failed != 0 {
		t.Fatalf("esperaba 0 fallidos en segunda ejecución, obtuve %d", failed)
	}
	_ = processed

	// La cuenta TB sigue existiendo, sin duplicados.
	exists, err := env.tb.AccountExists(tbID)
	if err != nil {
		t.Fatalf("AccountExists: %v", err)
	}
	if !exists {
		t.Fatal("TB debería tener la cuenta")
	}

	// El usuario sigue ACTIVE.
	fetched, err := env.pg.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if fetched.Status != models.StatusActive {
		t.Fatalf("status = %s, esperado ACTIVE", fetched.Status)
	}
}

func TestReconcileOnce_EmptyQueue(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Limpiar PENDINGs previos de otros tests.
	// Nota: los tests usan emails únicos, pero ListPendingUsers los
	// incluiría todos. Este test asume que la cola está vacía al
	// inicio o que los PENDINGs previos ya fueron procesados.
	processed, failed, err := env.reconciler.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}
	if failed != 0 {
		t.Fatalf("esperaba 0 fallidos, obtuve %d", failed)
	}
	t.Logf("procesados=%d (puede ser 0 si no había PENDINGs)", processed)
}
