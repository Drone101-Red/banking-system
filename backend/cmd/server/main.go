package main

import (
	"context"
	"encoding/json"
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

	txnService := transactions.NewService(pg, tbClient)
	txnHandler := transactions.NewHandler(txnService)

	acctService := account.NewService(pg, tbClient)
	acctHandler := account.NewHandler(acctService)

	// Wiring de chat
	openRouterClient := chat.NewOpenRouterClient(openRouterKey, openRouterModel)
	chatService := chat.NewService(pg, acctService, txnService, openRouterClient)
	chatHandler := chat.NewHandler(chatService)

	// Reconciliador
	reconciler := recovery.NewReconciler(pg, tbClient)
	go reconciler.Start(rootCtx, time.Duration(reconcileInterval)*time.Minute)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

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
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 90 * time.Second, // el chat puede tardar (LLM)
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
