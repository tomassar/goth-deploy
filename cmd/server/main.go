package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"deployer/internal/config"
	"deployer/internal/database"
	"deployer/internal/handlers"
	"deployer/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Load configuration
	cfg := config.Load()

	// Initialize database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	// Initialize services
	githubService := services.NewGitHubService(cfg.GitHubClientID, cfg.GitHubClientSecret)
	proxyService := services.NewProxyService()
	deploymentService := services.NewDeploymentService(cfg.DeploymentPath, cfg.BaseDomain, proxyService)
	secretsService := services.NewSecretsService(db, cfg.EncryptionKey)

	// Connect secrets service to deployment service
	deploymentService.SetSecretsService(secretsService)

	// Restart active deployments on startup
	log.Println("Restarting active deployments...")
	if err := deploymentService.RestartActiveDeployments(db); err != nil {
		log.Printf("Warning: Failed to restart some deployments: %v", err)
	} else {
		log.Println("Active deployments restarted successfully")
	}

	// Initialize handlers
	handler := handlers.NewTemplHandler(db, githubService, deploymentService, secretsService, cfg)

	// Setup main router for dashboard/management
	r := chi.NewRouter()

	// Setup subdomain router for deployed apps
	subdomainRouter := chi.NewRouter()

	// Middleware for main router
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// CORS for main router
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Main app routes (dashboard/management)
	r.Get("/", handler.Home)
	r.Get("/auth/github", handler.GitHubAuth)
	r.Get("/auth/github/callback", handler.GitHubCallback)
	r.Get("/dashboard", handler.Dashboard)
	r.Get("/projects", handler.ListProjects)
	r.Post("/projects", handler.CreateProject)
	r.Get("/projects/{id}", handler.GetProject)
	r.Post("/projects/{id}/deploy", handler.DeployProject)
	r.Post("/projects/{id}/stop", handler.StopProject)
	r.Delete("/projects/{id}", handler.DeleteProject)
	r.Get("/projects/{id}/logs", handler.GetDeploymentLogs)
	r.Get("/projects/{id}/secrets", handler.GetProjectSecrets)
	r.Post("/projects/{id}/secrets", handler.CreateProjectSecret)
	r.Put("/projects/{id}/secrets/{secretId}", handler.UpdateProjectSecret)
	r.Delete("/projects/{id}/secrets/{secretId}", handler.DeleteProjectSecret)
	r.Get("/projects/{id}/secrets/{secretId}/value", handler.GetSecretValue)
	r.Post("/deployments/stop-all", handler.StopAllProjects)
	r.Get("/deployments/active", handler.GetActiveDeployments)

	// Static files
	workDir, _ := os.Getwd()
	filesDir := http.Dir(workDir + "/web/static/")
	r.Handle("/static/*", http.StripPrefix("/static", http.FileServer(filesDir)))

	// Subdomain routes (deployed apps)
	subdomainRouter.Handle("/*", proxyService)

	// Create a custom handler that routes based on subdomain
	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		host := req.Host

		// Remove port from host
		if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
			host = host[:colonIndex]
		}

		// Check if this is a subdomain request
		parts := strings.Split(host, ".")
		if len(parts) > 1 && host != "localhost" && parts[0] != "" {
			// This is a subdomain request, use the proxy service
			subdomainRouter.ServeHTTP(w, req)
		} else {
			// This is the main app request
			r.ServeHTTP(w, req)
		}
	})

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mainHandler,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server starting on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Server failed to start:", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exited")
}
