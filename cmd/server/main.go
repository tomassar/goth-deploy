package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	deploymentService := services.NewDeploymentService(cfg.DeploymentPath, cfg.BaseDomain)

	// Initialize handlers
	handler := handlers.NewTemplHandler(db, githubService, deploymentService, cfg)

	// Setup router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Routes
	r.Get("/", handler.Home)
	r.Get("/auth/github", handler.GitHubAuth)
	r.Get("/auth/github/callback", handler.GitHubCallback)
	r.Get("/dashboard", handler.Dashboard)
	r.Get("/projects", handler.ListProjects)
	r.Post("/projects", handler.CreateProject)
	r.Get("/projects/{id}", handler.GetProject)
	r.Post("/projects/{id}/deploy", handler.DeployProject)
	r.Get("/projects/{id}/logs", handler.GetDeploymentLogs)
	r.Delete("/projects/{id}", handler.DeleteProject)

	// Static files
	workDir, _ := os.Getwd()
	filesDir := http.Dir(workDir + "/web/static/")
	r.Handle("/static/*", http.StripPrefix("/static", http.FileServer(filesDir)))

	// Start server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
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
