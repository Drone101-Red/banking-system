package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"time"

	"banking-system/internal/apperr"
	"banking-system/internal/auth"
	"banking-system/internal/transactions"
)

// ============================================================================
// Tipos del fixture
// ============================================================================

type fixtureData struct {
	Users        []fixtureUser        `json:"users"`
	Accounts     []fixtureAccount     `json:"accounts"`
	Transactions []fixtureTransaction `json:"transactions"`
}

type fixtureUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	FullName  string `json:"full_name"`
	CreatedAt string `json:"created_at"`
}

type fixtureAccount struct {
	AccountNumber  string  `json:"account_number"`
	UserID         string  `json:"user_id"`
	InitialBalance float64 `json:"initial_balance"`
	Currency       string  `json:"currency"`
	AccountType    string  `json:"account_type"`
}

type fixtureTransaction struct {
	FromAccount string  `json:"from_account"`
	ToAccount   string  `json:"to_account"`
	Amount      float64 `json:"amount"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Timestamp   string  `json:"timestamp"`
	Status      string  `json:"status"`
}

// ============================================================================
// Entrypoint
// ============================================================================

// seedFixtureUsers carga usuarios, saldos iniciales y transacciones históricas
// desde un archivo JSON de fixture.
//
// Solo en APP_ENV=development. Idempotente.
//
// Parámetros:
//   - path: ruta al archivo JSON
//   - userLimit: cuántos usuarios cargar (0 = todos)
//   - txnLimit: cuántas transacciones cargar en total (0 = todas)
func seedFixtureUsers(
	ctx context.Context,
	authSvc *auth.Service,
	txnSvc *transactions.Service,
	path string,
	userLimit int,
	txnLimit int,
) {
	if path == "" {
		return
	}

	// --- Fase 1: leer fixture ---
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[seed] no se pudo leer fixture %s: %v", path, err)
		return
	}

	var fixture fixtureData
	if err := json.Unmarshal(data, &fixture); err != nil {
		log.Printf("[seed] fixture inválido %s: %v", path, err)
		return
	}

	log.Printf("[seed] fixture leído: %d users, %d accounts, %d transactions",
		len(fixture.Users), len(fixture.Accounts), len(fixture.Transactions))

	// Colocar los saldos iniciales antes de cualquier transacción del fixture.
	seedStartTime := time.Now()
	for _, tx := range fixture.Transactions {
		t, err := time.Parse(time.RFC3339, tx.Timestamp)
		if err == nil && t.Before(seedStartTime) {
			seedStartTime = t
		}
	}
	seedStartTime = seedStartTime.Add(-time.Second)

	// --- Fase 2: cargar usuarios ---
	createdUsers := loadFixtureUsers(ctx, authSvc, fixture.Users, userLimit)
	userLimitEffective := effectiveLimit(userLimit, len(fixture.Users))
	log.Printf("[seed] usuarios cargados: %d nuevos de %d", createdUsers, userLimitEffective)

	loadedUserIDs := make(map[string]bool)
	fixtureToDBUserID := make(map[string]string)
	for _, u := range fixture.Users[:userLimitEffective] {
		loadedUserIDs[u.ID] = true
		pgUser, err := authSvc.GetUserByEmailForSeed(ctx, u.Email)
		if err != nil {
			log.Printf("[seed] no se pudo mapear usuario %s: %v", u.Email, err)
			continue
		}
		loadedUserIDs[pgUser.ID] = true
		fixtureToDBUserID[u.ID] = pgUser.ID
	}

	// --- Fase 3: construir mapas ---
	accountToUser := make(map[string]string)          // account_number → user_id real
	userAccounts := make(map[string][]fixtureAccount) // user_id → cuentas
	for _, acc := range fixture.Accounts {
		if dbUserID, ok := fixtureToDBUserID[acc.UserID]; ok {
			accountToUser[acc.AccountNumber] = dbUserID
		}
		userAccounts[acc.UserID] = append(userAccounts[acc.UserID], acc)
	}

	// --- Fase 4: cargar saldos iniciales ---
	loaded := loadInitialBalances(ctx, authSvc, txnSvc, fixture.Users, userAccounts, loadedUserIDs, seedStartTime)
	log.Printf("[seed] saldos iniciales cargados: %d", loaded)

	// --- Fase 5: cargar transacciones ---
	txnLoaded, txnSkipped, txnFailed := loadHistoricalTransactions(
		ctx, txnSvc, fixture.Transactions, accountToUser, loadedUserIDs, txnLimit,
	)
	log.Printf("[seed] transacciones: %d cargadas, %d filtradas, %d fallidas",
		txnLoaded, txnSkipped, txnFailed)
}

// ============================================================================
// Fase 2: Usuarios
// ============================================================================

func loadFixtureUsers(
	ctx context.Context,
	authSvc *auth.Service,
	users []fixtureUser,
	limit int,
) int {
	limit = effectiveLimit(limit, len(users))

	created := 0
	for _, u := range users[:limit] {
		_, err := authSvc.Register(ctx, auth.RegisterRequest{
			Email:    u.Email,
			Password: u.Password,
			FullName: u.FullName,
		})
		if err != nil {
			var appErr *apperr.AppError
			if errors.As(err, &appErr) && appErr.Code == "EMAIL_EXISTS" {
				continue
			}
			log.Printf("[seed] error creando usuario %s: %v", u.Email, err)
			continue
		}
		created++
	}
	return created
}

// ============================================================================
// Fase 4: Saldos iniciales
// ============================================================================

// loadInitialBalances crea un depósito por usuario con la suma de los saldos
// iniciales de todas sus cuentas en el fixture.
//
// Es idempotente: usa un Idempotency-Key determinístico por usuario.
func loadInitialBalances(
	ctx context.Context,
	authSvc *auth.Service,
	txnSvc *transactions.Service,
	users []fixtureUser,
	userAccounts map[string][]fixtureAccount,
	loadedUserIDs map[string]bool,
	seedStartTime time.Time,
) int {
	loaded := 0

	for _, u := range users {
		if !loadedUserIDs[u.ID] {
			continue
		}
		accounts := userAccounts[u.ID]
		if len(accounts) == 0 {
			continue
		}

		// Sumar los saldos iniciales
		var totalUSD float64
		for _, acc := range accounts {
			totalUSD += acc.InitialBalance
		}
		if totalUSD <= 0 {
			continue
		}

		totalCents := usdToCents(totalUSD)
		if totalCents <= 0 {
			continue
		}

		// Obtener el usuario de PostgreSQL (para tener el UUID interno)
		pgUser, err := authSvc.GetUserByEmailForSeed(ctx, u.Email)
		if err != nil {
			log.Printf("[seed] no se encontró usuario %s: %v", u.Email, err)
			continue
		}

		// Idempotency key fija por usuario
		key := fmt.Sprintf("seed-initial-%s", pgUser.ID)

		tx, err := txnSvc.Deposit(ctx, pgUser.ID, totalCents, key)
		if err != nil {
			if isIdempotentError(err) {
				continue
			}
			log.Printf("[seed] error depositando saldo inicial para %s: %v", u.Email, err)
			continue
		}

		if tx != nil {
			if err := txnSvc.UpdateTransactionMetadata(
				ctx,
				tx.TBTransferID,
				"Saldo inicial",
				seedStartTime,
			); err != nil {
				log.Printf("[seed] error actualizando metadata del saldo inicial: %v", err)
			}
		}
		loaded++
	}

	return loaded
}

// ============================================================================
// Fase 5: Transacciones históricas
// ============================================================================

// loadHistoricalTransactions carga las transacciones del fixture.
//
// Filtra:
//   - internal_transfer
//   - from_user == to_user
//   - cuentas inexistentes (excepto EXTERNAL)
//   - usuarios no cargados (fuera de userLimit)
//   - amount <= 0
//   - status != completed
//
// Ordena por timestamp ascendente.
// Es idempotente: usa un Idempotency-Key determinístico por transacción.
func loadHistoricalTransactions(
	ctx context.Context,
	txnSvc *transactions.Service,
	txs []fixtureTransaction,
	accountToUser map[string]string,
	loadedUserIDs map[string]bool,
	limit int,
) (loaded int, skipped int, failed int) {
	// Ordenar por timestamp ascendente
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Timestamp < txs[j].Timestamp
	})

	limit = effectiveLimit(limit, len(txs))

	for _, tx := range txs {
		if loaded >= limit {
			break
		}
		// Filtro: solo completed
		if tx.Status != "completed" {
			skipped++
			continue
		}

		// Filtro: internal_transfer
		if tx.Type == "internal_transfer" {
			skipped++
			continue
		}

		// Filtro: amount <= 0
		if tx.Amount <= 0 {
			skipped++
			continue
		}

		// Filtrar por tipo
		switch tx.Type {
		case "deposit", "withdrawal", "transfer":
			// OK
		default:
			skipped++
			continue
		}

		// Resolver usuario destino/source
		fromUserID, fromOK := resolveAccount(tx.FromAccount, accountToUser)
		toUserID, toOK := resolveAccount(tx.ToAccount, accountToUser)

		if !fromOK || !toOK {
			skipped++
			continue
		}

		// Ambos usuarios deben pertenecer al conjunto cargado.
		if fromUserID != "" && !loadedUserIDs[fromUserID] {
			skipped++
			continue
		}
		if toUserID != "" && !loadedUserIDs[toUserID] {
			skipped++
			continue
		}

		// Filtro: mismo usuario (incluye internal_transfer que no capturamos antes)
		if fromUserID == toUserID && fromUserID != "" {
			skipped++
			continue
		}

		// Ejecutar la transacción
		err := executeFixtureTransaction(ctx, txnSvc, tx, fromUserID, toUserID)
		if err != nil {
			if isIdempotentError(err) {
				skipped++
				continue
			}
			// Errores de saldo insuficiente son esperados en el fixture
			log.Printf("[seed] transacción rechazada (%s, %s, %.2f): %v",
				tx.FromAccount, tx.ToAccount, tx.Amount, err)
			failed++
			continue
		}
		loaded++
	}

	return loaded, skipped, failed
}

// resolveAccount convierte un account_number en un user_id.
//
// EXTERNAL no es un usuario: se representa con "" (string vacío).
func resolveAccount(accountNumber string, accountToUser map[string]string) (string, bool) {
	if accountNumber == "EXTERNAL" {
		return "", true
	}
	userID, ok := accountToUser[accountNumber]
	return userID, ok
}

// executeFixtureTransaction ejecuta una transacción del fixture según su tipo.
func executeFixtureTransaction(
	ctx context.Context,
	txnSvc *transactions.Service,
	tx fixtureTransaction,
	fromUserID string,
	toUserID string,
) error {
	amountCents := usdToCents(tx.Amount)
	if amountCents <= 0 {
		return fmt.Errorf("amount inválido")
	}

	// Parsear el timestamp del fixture
	fixtureTime, err := time.Parse(time.RFC3339, tx.Timestamp)
	if err != nil {
		return fmt.Errorf("timestamp inválido %q: %w", tx.Timestamp, err)
	}

	key := deterministicTransferID(tx)

	// Construir el "kind" del service
	var (
		kind        string
		userID      string
		toAccountID string
	)

	switch tx.Type {
	case "deposit":
		if toUserID == "" {
			return fmt.Errorf("deposit sin to_user")
		}
		kind = "deposit"
		userID = toUserID

	case "withdrawal":
		if fromUserID == "" {
			return fmt.Errorf("withdrawal sin from_user")
		}
		kind = "withdrawal"
		userID = fromUserID

	case "transfer":
		if fromUserID == "" || toUserID == "" {
			return fmt.Errorf("transfer sin from/to user")
		}
		kind = "transfer"
		userID = fromUserID
		// Obtener el tb_account_id del destinatario
		toTBAccountID, err := txnSvc.GetTBAccountIDByUserID(ctx, toUserID)
		if err != nil {
			return fmt.Errorf("get tb_account_id de %s: %w", toUserID, err)
		}
		toAccountID = toTBAccountID

	default:
		return fmt.Errorf("tipo desconocido: %s", tx.Type)
	}

	_, err = txnSvc.ExecuteFixtureTransaction(
		ctx,
		kind,
		userID,
		toAccountID,
		amountCents,
		key,
		tx.Description,
		fixtureTime,
	)
	return err
}

// ============================================================================
// Helpers
// ============================================================================

// usdToCents convierte un monto en USD a centavos.
//
// Usa math.Round para evitar errores de punto flotante.
func usdToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

// deterministicTransferID genera un ID determinístico para una transacción
// del fixture.
//
// Usa SHA256(from_account + "|" + to_account + "|" + amount + "|" + timestamp).
// El resultado se trunca a 16 bytes para que quepa en un uint128.
//
// La misma transacción produce siempre el mismo ID, así el seed es idempotente.
func deterministicTransferID(tx fixtureTransaction) string {
	h := sha256.New()
	h.Write([]byte(tx.FromAccount))
	h.Write([]byte("|"))
	h.Write([]byte(tx.ToAccount))
	h.Write([]byte("|"))
	h.Write([]byte(fmt.Sprintf("%.2f", tx.Amount)))
	h.Write([]byte("|"))
	h.Write([]byte(tx.Timestamp))

	sum := h.Sum(nil)
	return fmt.Sprintf("seed-%x", sum[:16])
}

// isIdempotentError devuelve true si el error indica que la operación
// ya fue ejecutada (idempotencia).
func isIdempotentError(err error) bool {
	var appErr *apperr.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "TRANSFER_EXISTS"
	}
	return false
}

// effectiveLimit normaliza un límite. Si es 0 o mayor que el total, devuelve
// el total. Si es negativo, devuelve 0.
func effectiveLimit(limit, total int) int {
	if limit <= 0 || limit > total {
		return total
	}
	return limit
}

// minInt devuelve el menor de dos enteros.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// notUsed evita el "declared and not used" si en el futuro se elimina algún import.
var _ = time.Now
