package services

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

type ProxyService struct {
	routes    map[string]string // subdomain -> target URL
	routesMux sync.RWMutex
}

func NewProxyService() *ProxyService {
	return &ProxyService{
		routes: make(map[string]string),
	}
}

// AddRoute adds a subdomain -> target URL mapping
func (p *ProxyService) AddRoute(subdomain, targetURL string) {
	p.routesMux.Lock()
	defer p.routesMux.Unlock()
	p.routes[subdomain] = targetURL
}

// RemoveRoute removes a subdomain mapping
func (p *ProxyService) RemoveRoute(subdomain string) {
	p.routesMux.Lock()
	defer p.routesMux.Unlock()
	delete(p.routes, subdomain)
}

// GetTargetURL returns the target URL for a subdomain
func (p *ProxyService) GetTargetURL(subdomain string) (string, bool) {
	p.routesMux.RLock()
	defer p.routesMux.RUnlock()
	target, exists := p.routes[subdomain]
	return target, exists
}

// ServeHTTP implements the http.Handler interface for reverse proxying
func (p *ProxyService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host

	// Extract subdomain from host
	subdomain := p.extractSubdomain(host)

	if subdomain == "" {
		// No subdomain, this should be handled by the main app
		http.Error(w, "No subdomain specified", http.StatusBadRequest)
		return
	}

	// Get target URL for this subdomain
	targetURL, exists := p.GetTargetURL(subdomain)
	if !exists {
		http.Error(w, fmt.Sprintf("Application '%s' not found or not deployed", subdomain), http.StatusNotFound)
		return
	}

	// Parse target URL
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Modify the request
	r.URL.Host = target.Host
	r.URL.Scheme = target.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = target.Host

	// Serve the request
	proxy.ServeHTTP(w, r)
}

// extractSubdomain extracts the subdomain from a host string
// For example: "my-app.localhost:8080" -> "my-app"
func (p *ProxyService) extractSubdomain(host string) string {
	// Remove port if present
	if colonIndex := strings.LastIndex(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	// Split by dots
	parts := strings.Split(host, ".")

	// If we have more than 1 part and it's not just "localhost"
	if len(parts) > 1 && host != "localhost" {
		// Return the first part as subdomain
		return parts[0]
	}

	return ""
}
