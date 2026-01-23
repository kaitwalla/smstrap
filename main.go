package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"telnyx-mock/internal/database"
	"telnyx-mock/internal/server"
)

//go:embed internal/ui/assets/*
var uiAssets embed.FS

func main() {
	// Initialize database
	dbPath := "smssink.db"
	if err := database.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.CloseDB()

	log.Println("Database initialized successfully")

	// Setup API server (port 23456)
	apiRouter := chi.NewRouter()
	apiRouter.Use(middleware.Logger)
	apiRouter.Use(middleware.Recoverer)
	apiRouter.Post("/v2/messages", server.HandleCreateMessage)
	apiRouter.Post("/v2/webhooks/messages", server.HandleInboundWebhook)

	apiServer := &http.Server{
		Addr:    ":23456",
		Handler: apiRouter,
	}

	// Setup UI server (port 23457)
	uiRouter := chi.NewRouter()
	uiRouter.Use(middleware.Logger)
	uiRouter.Use(middleware.Recoverer)

	// Serve the embedded HTML
	uiRouter.Get("/", func(w http.ResponseWriter, r *http.Request) {
		htmlContent, err := uiAssets.ReadFile("internal/ui/assets/index.html")
		if err != nil {
			http.Error(w, "Failed to load UI", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(htmlContent)
	})

	uiRouter.Get("/credentials", func(w http.ResponseWriter, r *http.Request) {
		htmlContent, err := uiAssets.ReadFile("internal/ui/assets/credentials.html")
		if err != nil {
			http.Error(w, "Failed to load credentials page", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(htmlContent)
	})

	// Serve static assets (logo, favicons)
	uiRouter.Get("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		content, err := uiAssets.ReadFile("internal/ui/assets/logo.png")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(content)
	})

	uiRouter.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		content, err := uiAssets.ReadFile("internal/ui/assets/favicon-32x32.png")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(content)
	})

	uiRouter.Get("/favicon-{size}.png", func(w http.ResponseWriter, r *http.Request) {
		size := chi.URLParam(r, "size")
		filename := "internal/ui/assets/favicon-" + size + ".png"
		content, err := uiAssets.ReadFile(filename)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(content)
	})

	uiRouter.Get("/apple-touch-icon.png", func(w http.ResponseWriter, r *http.Request) {
		content, err := uiAssets.ReadFile("internal/ui/assets/apple-touch-icon.png")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(content)
	})

	uiRouter.Get("/android-chrome-{size}.png", func(w http.ResponseWriter, r *http.Request) {
		size := chi.URLParam(r, "size")
		filename := "internal/ui/assets/android-chrome-" + size + ".png"
		content, err := uiAssets.ReadFile(filename)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(content)
	})

	uiRouter.Get("/site.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		content, err := uiAssets.ReadFile("internal/ui/assets/site.webmanifest")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Write(content)
	})

	// Serve the logs page
	uiRouter.Get("/logs", func(w http.ResponseWriter, r *http.Request) {
		htmlContent, err := uiAssets.ReadFile("internal/ui/assets/logs.html")
		if err != nil {
			http.Error(w, "Failed to load logs page", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(htmlContent)
	})

	// API endpoints for UI
	uiRouter.Get("/api/messages", server.HandleListMessages)
	uiRouter.Delete("/api/messages", server.HandleClearMessages)
	uiRouter.Post("/api/messages/inbound", server.HandleSimulateInbound)
	uiRouter.Get("/api/credentials", server.HandleGetCredentials)
	uiRouter.Post("/api/credentials", server.HandleSetCredentials)
	uiRouter.Get("/api/logs", server.HandleGetLogs)
	uiRouter.Delete("/api/logs", server.HandleClearLogs)

	uiServer := &http.Server{
		Addr:    ":23457",
		Handler: uiRouter,
	}

	// Start API server
	go func() {
		log.Printf("API server starting on port 23456")
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	// Start UI server
	go func() {
		log.Printf("UI server starting on port 23457")
		if err := uiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("UI server failed: %v", err)
		}
	}()

	log.Println("Telnyx Mock Server is running")
	log.Println("API endpoint: http://localhost:23456/v2/messages")
	log.Println("Web UI: http://localhost:23457")

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down servers...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down API server: %v", err)
	}

	if err := uiServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down UI server: %v", err)
	}

	log.Println("Servers stopped")
}
