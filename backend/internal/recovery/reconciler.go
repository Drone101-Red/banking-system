// Package recovery implementa la reconciliación de usuarios PENDING.
//
// Cuando el registro de un usuario falla al crear su cuenta en
// TigerBeetle (por ejemplo, TB estaba caído), el usuario queda en
// PostgreSQL con status=PENDING. El reconciliador recupera esos
// usuarios en el siguiente ciclo.
//
// Reglas:
//   - PostgreSQL es la autoridad de identidad. Nunca crear usuarios
//     a partir de cuentas en TB.
//   - Una caída temporal de TB deja al usuario PENDING, no FAILED.
//   - La transición PENDING -> ACTIVE es idempotente.
package recovery

import (
	"context"
	"errors"
	"log"
	"time"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
)

// BatchSize es el número máximo de usuarios PENDING procesados por ciclo.
const BatchSize = 100

// TBAccountManager abstrae las operaciones de TB que necesita el
// reconciliador. Permite mockear en tests unitarios.
type TBAccountManager interface {
	AccountExists(id tb.Uint128) (bool, error)
	CreateAccount(id tb.Uint128, code uint16) error
}

// Reconciler recupera usuarios PENDING.
type Reconciler struct {
	pg *db.PostgresStore
	tb TBAccountManager
}

// NewReconciler construye el Reconciler.
func NewReconciler(pg *db.PostgresStore, tb TBAccountManager) *Reconciler {
	return &Reconciler{pg: pg, tb: tb}
}

// ReconcileOnce procesa un lote de hasta BatchSize usuarios PENDING.
//
// Devuelve:
//   - processed: cuántos usuarios se procesaron (independientemente del resultado).
//   - failed: cuántos fallaron (TB no disponible o error de DB).
//   - err: error de nivel ciclo (por ejemplo, no se pudo leer la lista).
//
// Es idempotente: correrlo dos veces sobre el mismo usuario no produce
// efectos secundarios adicionales.
//
// Es seguro ante concurrencia: UpdateUserStatusActive solo actualiza
// filas con status=PENDING. Si dos ejecuciones procesan al mismo usuario,
// la segunda verá 0 filas afectadas y no fallará.
func (r *Reconciler) ReconcileOnce(ctx context.Context) (processed, failed int, err error) {
	users, err := r.pg.ListPendingUsers(ctx, BatchSize)
	if err != nil {
		return 0, 0, err
	}

	if len(users) == 0 {
		return 0, 0, nil
	}

	log.Printf("[recovery] procesando %d usuarios PENDING", len(users))

	for _, u := range users {
		if ctx.Err() != nil {
			return processed, failed, ctx.Err()
		}

		if err := r.reconcileUser(ctx, u); err != nil {
			failed++
			log.Printf("[recovery] usuario %s falló: %v", u.ID, err)
			continue
		}
		processed++
	}

	log.Printf("[recovery] ciclo: %d OK, %d fallidos", processed, failed)
	return processed, failed, nil
}

// reconcileUser intenta llevar a un usuario PENDING a ACTIVE.
//
// Flujo:
//  1. Verificar si la cuenta TB existe.
//  2. Si no existe, crearla (idempotente).
//  3. Marcar el usuario como ACTIVE (idempotente).
//
// Si TB no está disponible, devuelve error y el usuario permanece PENDING.
func (r *Reconciler) reconcileUser(ctx context.Context, u *models.User) error {
	if len(u.TBAccountID) != 16 {
		return apperr.Internal(
			errors.New("TBAccountID debe ser 16 bytes"),
		)
	}

	tbID := tb.BytesToUint128([16]byte(u.TBAccountID))

	exists, err := r.tb.AccountExists(tbID)
	if err != nil {
		// TB no disponible (o error de red). No es fatal:
		// el usuario permanece PENDING y se reintenta en el próximo ciclo.
		return err
	}

	if !exists {
		// Crear la cuenta. CreateAccount es idempotente: si otra
		// ejecución la creó entre el AccountExists y el CreateAccount,
		// no falla (devuelve AccountExists).
		if err := r.tb.CreateAccount(tbID, models.CodeSavings); err != nil {
			return err
		}
	}

	// Marcar ACTIVE. Idempotente vía UpdateUserStatusActive.
	if err := r.pg.UpdateUserStatusActive(ctx, u.ID); err != nil {
		return err
	}

	return nil
}

// Start lanza el reconciliador en background.
//
// Corre una vez inmediatamente al arrancar (por si quedaron usuarios
// PENDING del arranque anterior) y luego cada `interval`.
//
// Retorna cuando ctx se cancela. Usar con `go reconciler.Start(ctx, interval)`
// desde main.
func (r *Reconciler) Start(ctx context.Context, interval time.Duration) {
	log.Printf("[recovery] iniciando (intervalo: %v)", interval)

	// Primera ejecución inmediata.
	r.runCycle(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[recovery] detenido")
			return
		case <-ticker.C:
			r.runCycle(ctx)
		}
	}
}

// runCycle ejecuta ReconcileOnce y registra el resultado.
// Nunca hace panic ni detiene el reconciliador.
func (r *Reconciler) runCycle(ctx context.Context) {
	processed, failed, err := r.ReconcileOnce(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("[recovery] error de ciclo: %v", err)
		return
	}
	if processed == 0 && failed == 0 {
		return
	}
	log.Printf("[recovery] procesados=%d fallidos=%d", processed, failed)
}
