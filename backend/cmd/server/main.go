package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"banking-system/internal/account"
	"banking-system/internal/apperr"
	"banking-system/internal/auth"
	"banking-system/internal/chat"
	"banking-system/internal/db"
	"banking-system/internal/models"
	"banking-system/internal/recovery"
	"banking-system/internal/tigerbeetle"
	"banking-system/internal/transactions"
)

func main() {
	pgHost := getEnv("POSTGRES_HOST", "postgres")
	pgPort := getEnv("POSTGRES_PORT", "5432")
	pgUser := getEnv("POSTGRES_USER", "banking_user")
	pgPass := getEnv("POSTGRES_PASSWORD", "secret123")
	pgDB := getEnv("POSTGRES_DB", "banking")
	port := getEnv("APP_PORT", "8080")
	tbAddress := getEnv("TB_ADDRESS", "tigerbeetle:3000")
	tbClusterID := getEnv("TB_CLUSTER_ID", "0")

	jwtSecret := getEnv("JWT_SECRET", "")
	jwtExpiryHours := getEnvInt("JWT_EXPIRY_HOURS", 24)
	reconcileInterval := getEnvInt("RECONCILE_INTERVAL_MINUTES", 5)

	if jwtSecret == "" {
		log.Fatal("❌ JWT_SECRET no configurado. Definir en .env")
	}
	if len(jwtSecret) < 32 {
		log.Fatal("❌ JWT_SECRET debe tener al menos 32 caracteres")
	}

	openRouterKey := getEnv("OPENROUTER_API_KEY", "")
	if openRouterKey == "" {
		log.Fatal("❌ OPENROUTER_API_KEY no configurada. Definir en .env")
	}
	openRouterModel := getEnv("OPENROUTER_MODEL", "openrouter/free")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		pgHost, pgPort, pgUser, pgPass, pgDB,
	)

	pg, err := db.NewPostgresStore(dsn)
	if err != nil {
		log.Fatalf("❌ PostgreSQL: %v", err)
	}
	defer pg.Close()
	log.Println("✅ PostgreSQL conectado")

	tbClient, err := tigerbeetle.New(tbAddress, tbClusterID)
	if err != nil {
		log.Fatalf("❌ TigerBeetle: %v", err)
	}
	defer tbClient.Close()

	if err := tbClient.Ping(); err != nil {
		log.Fatalf("❌ TigerBeetle: %v", err)
	}
	log.Println("✅ TigerBeetle conectado")

	if err := tbClient.EnsureBankAccount(); err != nil {
		log.Fatalf("❌ Cuenta banco: %v", err)
	}
	log.Println("✅ Cuenta banco verificada (ID=1)")

	// Context raíz para servicios de background. Se cancela al shutdown.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Wiring de servicios
	jwtExpiry := time.Duration(jwtExpiryHours) * time.Hour
	authService := auth.NewService(pg, tbClient, jwtSecret, jwtExpiry)
	authHandler := auth.NewHandler(authService)

	// Seed de usuarios demo (solo en development)
	if os.Getenv("APP_ENV") == "development" {
		seedDemoUsers(rootCtx, authService)
		seedFixtureUsers(rootCtx, authService, os.Getenv("TEST_DATA_FILE"), getEnvInt("TEST_DATA_LIMIT", 100))
	}

	txnService := transactions.NewService(pg, tbClient)
	txnHandler := transactions.NewHandler(txnService)

	acctService := account.NewService(pg, tbClient)
	acctHandler := account.NewHandler(acctService)

	// Wiring de chat
	openRouterClient := chat.NewOpenRouterClient(openRouterKey, openRouterModel)
	confirmStore := chat.NewPostgresConfirmationStore(pg)
	chatService := chat.NewService(pg, acctService, txnService, openRouterClient, confirmStore)
	chatHandler := chat.NewHandler(chatService)

	// Reconciliador
	reconciler := recovery.NewReconciler(pg, tbClient)
	go reconciler.Start(rootCtx, time.Duration(reconcileInterval)*time.Minute)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(90 * time.Second))

	r.Get("/health", healthHandler(pg, tbClient))

	// Rutas de auth
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/register", authHandler.RegisterHandler)
		r.Post("/login", authHandler.LoginHandler)
		r.Post("/logout", authHandler.LogoutHandler)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireAuth(jwtSecret))
			r.Get("/me", authHandler.MeHandler)
		})
	})

	// Rutas de transacciones (requieren JWT)
	r.Route("/api/transactions", func(r chi.Router) {
		r.Use(auth.RequireAuth(jwtSecret))
		r.Post("/deposit", txnHandler.DepositHandler)
		r.Post("/withdraw", txnHandler.WithdrawHandler)
		r.Post("/transfer", txnHandler.TransferHandler)
		r.Get("/history", txnHandler.HistoryHandler)
		r.Post("/demo-topup", txnHandler.DemoTopupHandler)
	})

	// Rutas de cuenta (requieren JWT)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(jwtSecret))
		r.Get("/api/account", acctHandler.InfoHandler)
		r.Get("/api/account/balance", acctHandler.BalanceHandler)
	})

	// Ruta de chat (requiere JWT)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(jwtSecret))
		r.Post("/api/chat", chatHandler.ChatHandler)
		r.Post("/api/chat/confirm", chatHandler.ConfirmHandler)
	})

	// Ruta de lookup de usuarios (requiere JWT)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth(jwtSecret))
		r.Get("/api/users/lookup", authHandler.LookupHandler)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 Servidor en :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Cerrando...")

	rootCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// seedDemoUsers crea los usuarios demo si no existen.
//
// Solo en APP_ENV=development. Idempotente: si el email ya existe,
// continúa sin error.
//
// Usuarios creados:
//   - demo@banco.com / Demo1234!
//   - demo2@banco.com / Demo1234!
//
// Estos usuarios se crean con alias (demo, demo2) y con su cuenta
// en TigerBeetle, igual que cualquier usuario registrado vía la API.
func seedDemoUsers(ctx context.Context, svc *auth.Service) {
	demos := []auth.RegisterRequest{
		{Email: "demo@banco.com", Password: "Demo1234!", FullName: "Usuario Demo"},
		{Email: "demo2@banco.com", Password: "Demo1234!", FullName: "Usuario Demo 2"},
	}
	for _, d := range demos {
		_, err := svc.Register(ctx, d)
		if err != nil {
			var appErr *apperr.AppError
			if errors.As(err, &appErr) && appErr.Code == "EMAIL_EXISTS" {
				continue // ya existe, OK
			}
			log.Printf("[seed] error creando %s: %v", d.Email, err)
			continue
		}
		log.Printf("[seed] usuario demo creado: %s", d.Email)
	}
}

type fixtureData struct {
	Users []struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
	} `json:"users"`
}

func seedFixtureUsers(ctx context.Context, svc *auth.Service, path string, limit int) {
	if path == "" || limit <= 0 {
		return
	}

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

	if limit > len(fixture.Users) {
		limit = len(fixture.Users)
	}

	created := 0
	for _, user := range fixture.Users[:limit] {
		_, err := svc.Register(ctx, auth.RegisterRequest{
			Email:    user.Email,
			Password: user.Password,
			FullName: user.FullName,
		})
		if err != nil {
			var appErr *apperr.AppError
			if errors.As(err, &appErr) && appErr.Code == "EMAIL_EXISTS" {
				continue
			}
			log.Printf("[seed] error creando %s: %v", user.Email, err)
			continue
		}
		created++
	}

	log.Printf("[seed] fixture cargado: %d usuarios nuevos de %d", created, limit)
}

func healthHandler(pg *db.PostgresStore, tbClient *tigerbeetle.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		pgStatus := "ok"
		if err := pg.Ping(ctx); err != nil {
			pgStatus = "error"
		}

		tbStatus := "ok"
		bankID := tb.ToUint128(models.BankAccountID)
		exists, err := tbClient.AccountExists(bankID)
		if err != nil || !exists {
			tbStatus = "error"
		}

		status := "healthy"
		httpStatus := http.StatusOK

		if pgStatus != "ok" || tbStatus != "ok" {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":      status,
			"postgres":    pgStatus,
			"tigerbeetle": tbStatus,
		})
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getEnvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("⚠️  %s no es un entero válido (%q), usando default %d", k, v, def)
		return def
	}
	return n
}
