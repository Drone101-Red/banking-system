package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"banking-system/internal/db"
	"banking-system/internal/tigerbeetle"
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

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))

	r.Get("/health", healthHandler(pg, tbClient))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// healthHandler devuelve el estado de PostgreSQL.
// En el paso 2.4 se amplia para incluir TigerBeetle.
func healthHandler(pg *db.PostgresStore, tbClient *tigerbeetle.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		pgStatus := "ok"
		if err := pg.Ping(ctx); err != nil {
			pgStatus = "error"
		}

		tbStatus := "ok"
		if err := tbClient.Ping(); err != nil {
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
