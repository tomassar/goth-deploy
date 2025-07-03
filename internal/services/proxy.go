package services

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"goth-deploy/internal/config"
	"goth-deploy/internal/models"
)

// ProxyService handles reverse proxy for deployed applications
type ProxyService struct {
	DB         *sql.DB
	Config     *config.Config
	Deployment *DeploymentService
	proxies    map[string]*httputil.ReverseProxy
	mutex      sync.RWMutex
}

// NewProxyService creates a new proxy service
func NewProxyService(db *sql.DB, cfg *config.Config) *ProxyService {
	return &ProxyService{
		DB:      db,
		Config:  cfg,
		proxies: make(map[string]*httputil.ReverseProxy),
	}
}

// SetDeploymentService sets the deployment service reference
func (p *ProxyService) SetDeploymentService(deployment *DeploymentService) {
	p.Deployment = deployment
}

// ServeHTTP handles incoming requests and routes them to the appropriate application
func (p *ProxyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract subdomain from host
	subdomain := p.extractSubdomain(r.Host)
	if subdomain == "" {
		http.Error(w, "Invalid subdomain", http.StatusBadRequest)
		return
	}

	// Special case: main domain or www should not be proxied
	if subdomain == "www" || subdomain == p.Config.BaseDomain {
		http.Error(w, "Invalid subdomain", http.StatusBadRequest)
		return
	}

	// Get project by subdomain
	project, err := p.getProjectBySubdomain(subdomain)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Check if project is active
	if project.Status != models.ProjectStatusActive {
		// Try to start the application if deployment service is available
		if p.Deployment != nil && project.Status == models.ProjectStatusInactive {
			if err := p.Deployment.RestartProject(project.ID); err != nil {
				http.Error(w, "Project is not available", http.StatusServiceUnavailable)
				return
			}
			// Wait a moment for the app to start
			// Note: In production, you might want to implement a more sophisticated waiting mechanism
		} else {
			http.Error(w, "Project is not active", http.StatusServiceUnavailable)
			return
		}
	}

	// Ensure the application is running
	if p.Deployment != nil {
		if !p.Deployment.IsProjectRunning(subdomain) {
			if err := p.Deployment.RestartProject(project.ID); err != nil {
				http.Error(w, "Failed to start application", http.StatusInternalServerError)
				return
			}
		}
	}

	// Get or create reverse proxy
	proxy := p.getOrCreateProxy(subdomain, project.Port)
	if proxy == nil {
		http.Error(w, "Failed to create proxy", http.StatusInternalServerError)
		return
	}

	// Proxy the request
	proxy.ServeHTTP(w, r)
}

// extractSubdomain extracts the subdomain from the host header
func (p *ProxyService) extractSubdomain(host string) string {
	// Remove port if present
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	// Split by dots
	parts := strings.Split(host, ".")

	// For localhost development, the subdomain is the first part before localhost
	if len(parts) >= 2 && (parts[len(parts)-1] == "localhost" || strings.Contains(host, "localhost")) {
		return parts[0]
	}

	// For production domains like example.com, extract subdomain
	baseParts := strings.Split(p.Config.BaseDomain, ".")
	if len(parts) > len(baseParts) {
		// Return the subdomain part (first part)
		return parts[0]
	}

	return ""
}

// getProjectBySubdomain retrieves a project by its subdomain
func (p *ProxyService) getProjectBySubdomain(subdomain string) (*models.Project, error) {
	var project models.Project
	err := p.DB.QueryRow(`
		SELECT id, user_id, name, github_repo_id, repo_url, branch, subdomain, 
		       build_command, start_command, port, status, last_deploy, created_at, updated_at
		FROM projects WHERE subdomain = ?
	`, subdomain).Scan(
		&project.ID,
		&project.UserID,
		&project.Name,
		&project.GitHubRepoID,
		&project.RepoURL,
		&project.Branch,
		&project.Subdomain,
		&project.BuildCommand,
		&project.StartCommand,
		&project.Port,
		&project.Status,
		&project.LastDeploy,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// getOrCreateProxy gets or creates a reverse proxy for the given subdomain and port
func (p *ProxyService) getOrCreateProxy(subdomain string, port int) *httputil.ReverseProxy {
	p.mutex.RLock()
	if proxy, exists := p.proxies[subdomain]; exists {
		p.mutex.RUnlock()
		return proxy
	}
	p.mutex.RUnlock()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Double-check pattern
	if proxy, exists := p.proxies[subdomain]; exists {
		return proxy
	}

	// Create new proxy
	target, err := url.Parse(fmt.Sprintf("http://localhost:%d", port))
	if err != nil {
		fmt.Printf("Failed to parse proxy target URL: %v\n", err)
		return nil
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize proxy error handling
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		fmt.Printf("Proxy error for %s: %v\n", subdomain, err)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`
			<!DOCTYPE html>
			<html>
			<head>
				<title>Application Unavailable</title>
				<style>
					body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; 
						   display: flex; align-items: center; justify-content: center; min-height: 100vh; 
						   margin: 0; background: #f3f4f6; }
					.container { text-align: center; background: white; padding: 2rem; border-radius: 8px; 
								box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1); }
					h1 { color: #dc2626; margin-bottom: 1rem; }
					p { color: #6b7280; margin: 0.5rem 0; }
					.code { background: #f9fafb; padding: 0.25rem 0.5rem; border-radius: 4px; 
						   font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace; font-size: 0.875rem; }
				</style>
			</head>
			<body>
				<div class="container">
					<h1>🚀 Application Starting</h1>
					<p>The application is currently starting up.</p>
					<p>Please refresh the page in a few moments.</p>
					<p class="code">` + subdomain + `</p>
				</div>
			</body>
			</html>
		`))
	}

	// Store the proxy
	p.proxies[subdomain] = proxy
	return proxy
}

// RemoveProxy removes a proxy for a subdomain (useful when project is deleted)
func (p *ProxyService) RemoveProxy(subdomain string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	delete(p.proxies, subdomain)
}

// HealthCheck checks if the proxy service is working
func (p *ProxyService) HealthCheck() error {
	// Check database connection
	if err := p.DB.Ping(); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	return nil
}

// GetActiveProjects returns a list of currently proxied projects
func (p *ProxyService) GetActiveProjects() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	var subdomains []string
	for subdomain := range p.proxies {
		subdomains = append(subdomains, subdomain)
	}
	return subdomains
}
