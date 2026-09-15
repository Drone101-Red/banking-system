// Package account implementa las consultas sobre la cuenta del usuario.
//
// A diferencia de transactions, estas operaciones no modifican estado:
// solo leen de TigerBeetle.
package account

import (
	"context"

	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/apperr"
	"banking-system/internal/db"
	"banking-system/internal/models"
)

// TBClient abstrae las operaciones de TigerBeetle que necesita el Service.
type TBClient interface {
	GetBalance(id tb.Uint128) (int64, error)
	GetAccountInfo(id tb.Uint128) (*models.AccountInfo, error)
}

// Service orquesta las consultas de cuenta.
type Service struct {
	pg *db.PostgresStore
	tb TBClient
}

// NewService construye el Service.
func NewService(pg *db.PostgresStore, tb TBClient) *Service {
	return &Service{pg: pg, tb: tb}
}

// Info devuelve la información completa de la cuenta del usuario.
func (s *Service) Info(ctx context.Context, userID string) (*models.AccountInfo, error) {
	user, err := s.pg.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Status != models.StatusActive {
		return nil, apperr.Forbidden("USER_NOT_ACTIVE", "La cuenta no está activa")
	}

	var arr [16]byte
	copy(arr[:], user.TBAccountID)
	tbID := tb.BytesToUint128(arr)

	return s.tb.GetAccountInfo(tbID)
}

// Balance devuelve solo el saldo en centavos.
func (s *Service) Balance(ctx context.Context, userID string) (int64, error) {
	user, err := s.pg.GetUserByID(ctx, userID)
	if err != nil {
		return 0, err
	}
	if user.Status != models.StatusActive {
		return 0, apperr.Forbidden("USER_NOT_ACTIVE", "La cuenta no está activa")
	}

	var arr [16]byte
	copy(arr[:], user.TBAccountID)
	tbID := tb.BytesToUint128(arr)

	return s.tb.GetBalance(tbID)
}
