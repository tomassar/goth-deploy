package main

import (
	"log"
	"net/http"
	"strings"

	"goth-deploy/internal/config"
	"goth-deploy/internal/database"
	"goth-deploy/internal/handlers"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize configuration
	cfg := config.New()

	// Initialize database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Run migrations
	if err := database.Migrate(db); err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	// Initialize handlers
	handler := handlers.New(db, cfg)

	// Create a custom server that routes based on subdomains
	server := &subdomainRouter{
		mainHandler:  handler.Routes(),
		proxyHandler: handler.Proxy,
		baseDomain:   cfg.BaseDomain,
	}

	// Start server
	log.Printf("Starting server on :%s", cfg.Port)
	log.Printf("Main application: http://%s", cfg.BaseDomain)
	log.Printf("Deployed apps: http://{subdomain}.%s", cfg.BaseDomain)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, server))
}

// subdomainRouter routes requests based on subdomain
type subdomainRouter struct {
	mainHandler  http.Handler
	proxyHandler http.Handler
	baseDomain   string
}

func (sr *subdomainRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host

	// Remove port if present
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	// Check if this is a subdomain request
	if sr.isSubdomainRequest(host) {
		// Route to proxy for deployed applications
		sr.proxyHandler.ServeHTTP(w, r)
	} else {
		// Route to main application
		sr.mainHandler.ServeHTTP(w, r)
	}
}

func (sr *subdomainRouter) isSubdomainRequest(host string) bool {
	// For localhost development
	if strings.Contains(host, "localhost") {
		parts := strings.Split(host, ".")
		// If there's a subdomain before localhost (e.g., myapp.localhost)
		return len(parts) > 1 && parts[0] != "www"
	}

	// For production domains
	baseDomain := sr.baseDomain
	if colonIndex := strings.Index(baseDomain, ":"); colonIndex != -1 {
		baseDomain = baseDomain[:colonIndex]
	}

	// Check if host has more parts than base domain (indicating a subdomain)
	hostParts := strings.Split(host, ".")
	baseParts := strings.Split(baseDomain, ".")

	// If host has more parts than base domain, it's likely a subdomain
	if len(hostParts) > len(baseParts) {
		return true
	}

	// Check if host is exactly the base domain
	return host != baseDomain && host != "www."+baseDomain
}
