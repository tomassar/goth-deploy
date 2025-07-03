package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"goth-deploy/web/templates"

	"github.com/go-chi/chi/v5"
)

// ProjectsListHandler lists all projects for the authenticated user
func (h *Handler) ProjectsListHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// For now, just redirect to dashboard
	// TODO: Create a dedicated projects list template
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// NewProjectHandler shows the new project form
func (h *Handler) NewProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Render the new project template
	component := templates.NewProject(user)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering new project template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// CreateProjectHandler creates a new project
func (h *Handler) CreateProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	log.Printf("CreateProject request from user %s", user.Username)
	log.Printf("Content-Type: %s", r.Header.Get("Content-Type"))
	log.Printf("Request Method: %s", r.Method)

	// Parse form data - handle both URL-encoded and multipart forms
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB max memory
		// If multipart parsing fails, try regular form parsing
		if err := r.ParseForm(); err != nil {
			log.Printf("Error parsing form (both multipart and regular): %v", err)
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		log.Printf("Successfully parsed as URL-encoded form")
	} else {
		log.Printf("Successfully parsed as multipart form")
	}

	// Log all form values for debugging
	log.Printf("All form values received:")
	for key, values := range r.Form {
		log.Printf("  %s: %v", key, values)
	}

	// Extract and validate form data
	name := strings.TrimSpace(r.FormValue("name"))
	githubRepoIDStr := r.FormValue("github_repo_id")
	repoURL := strings.TrimSpace(r.FormValue("repo_url"))
	branch := strings.TrimSpace(r.FormValue("branch"))
	subdomain := strings.TrimSpace(r.FormValue("subdomain"))
	buildCommand := strings.TrimSpace(r.FormValue("build_command"))
	startCommand := strings.TrimSpace(r.FormValue("start_command"))
	portStr := r.FormValue("port")

	// Log each field after extraction and trimming
	log.Printf("Extracted form fields:")
	log.Printf("  name: '%s' (len=%d)", name, len(name))
	log.Printf("  github_repo_id: '%s' (len=%d)", githubRepoIDStr, len(githubRepoIDStr))
	log.Printf("  repo_url: '%s' (len=%d)", repoURL, len(repoURL))
	log.Printf("  branch: '%s' (len=%d)", branch, len(branch))
	log.Printf("  subdomain: '%s' (len=%d)", subdomain, len(subdomain))
	log.Printf("  build_command: '%s' (len=%d)", buildCommand, len(buildCommand))
	log.Printf("  start_command: '%s' (len=%d)", startCommand, len(startCommand))
	log.Printf("  port: '%s' (len=%d)", portStr, len(portStr))

	// Validate required fields
	if name == "" || githubRepoIDStr == "" || repoURL == "" || branch == "" || subdomain == "" || buildCommand == "" || startCommand == "" || portStr == "" {
		log.Printf("Validation failed - missing required fields:")
		if name == "" {
			log.Printf("  - name is empty")
		}
		if githubRepoIDStr == "" {
			log.Printf("  - github_repo_id is empty")
		}
		if repoURL == "" {
			log.Printf("  - repo_url is empty")
		}
		if branch == "" {
			log.Printf("  - branch is empty")
		}
		if subdomain == "" {
			log.Printf("  - subdomain is empty")
		}
		if buildCommand == "" {
			log.Printf("  - build_command is empty")
		}
		if startCommand == "" {
			log.Printf("  - start_command is empty")
		}
		if portStr == "" {
			log.Printf("  - port is empty")
		}
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	log.Printf("✅ All required fields validation passed!")

	// Parse numeric fields
	githubRepoID, err := strconv.ParseInt(githubRepoIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid GitHub repository ID", http.StatusBadRequest)
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "Invalid port number", http.StatusBadRequest)
		return
	}

	// Validate subdomain format
	if !isValidSubdomain(subdomain) {
		http.Error(w, "Invalid subdomain format", http.StatusBadRequest)
		return
	}

	// Check if subdomain is already taken
	if exists, err := h.subdomainExists(subdomain); err != nil {
		log.Printf("Error checking subdomain: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	} else if exists {
		http.Error(w, "Subdomain already exists", http.StatusConflict)
		return
	}

	// Generate a unique port for this project
	projectPort, err := h.generateUniquePort()
	if err != nil {
		log.Printf("Error generating unique port: %v", err)
		// Fall back to provided port if generation fails
		projectPort = port
	}

	// Create project in database
	result, err := h.DB.Exec(`
		INSERT INTO projects (
			user_id, name, github_repo_id, repo_url, branch, subdomain, 
			build_command, start_command, port, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'inactive', ?, ?)
	`, user.ID, name, githubRepoID, repoURL, branch, subdomain, buildCommand, startCommand, projectPort, time.Now(), time.Now())

	if err != nil {
		log.Printf("Error creating project: %v", err)
		http.Error(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	projectID, err := result.LastInsertId()
	if err != nil {
		log.Printf("Error getting project ID: %v", err)
		http.Error(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	log.Printf("Created project %d for user %s: %s", projectID, user.Username, name)

	// Trigger initial deployment
	deployment, err := h.Deployment.DeployProject(projectID, "")
	if err != nil {
		log.Printf("Error starting initial deployment: %v", err)
		// Don't return error here, project was created successfully
	} else {
		log.Printf("Started initial deployment %d for project %d", deployment.ID, projectID)
	}

	// Return success response
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"message":    "Project created successfully",
			"project_id": projectID,
		})
		return
	}

	// For regular requests, redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ProjectDetailsHandler shows details for a specific project
func (h *Handler) ProjectDetailsHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	projectID := chi.URLParam(r, "id")
	log.Printf("Project details request for project %s from user %s", projectID, user.Username)

	// TODO: Create project details template
	w.Write([]byte(`
		<div class="max-w-4xl mx-auto py-8">
			<h2 class="text-2xl font-bold mb-6">Project Details</h2>
			<p class="text-gray-600">Project ID: ` + projectID + `</p>
			<p class="text-gray-600">Project details coming soon...</p>
			<a href="/dashboard" class="text-purple-600 hover:text-purple-500">← Back to Dashboard</a>
		</div>
	`))
}

// UpdateProjectHandler updates a project
func (h *Handler) UpdateProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	projectID := chi.URLParam(r, "id")
	log.Printf("Update project request for project %s from user %s", projectID, user.Username)

	// TODO: Implement project update
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// DeleteProjectHandler deletes a project
func (h *Handler) DeleteProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	projectIDStr := chi.URLParam(r, "id")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		log.Printf("Invalid project ID: %v", err)
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Verify user owns this project
	var ownerID int64
	err = h.DB.QueryRow("SELECT user_id FROM projects WHERE id = ?", projectID).Scan(&ownerID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Project not found", http.StatusNotFound)
		} else {
			log.Printf("Error checking project ownership: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	if ownerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete the project using deployment service
	if err := h.Deployment.DeleteProject(projectID); err != nil {
		log.Printf("Error deleting project: %v", err)
		http.Error(w, "Failed to delete project", http.StatusInternalServerError)
		return
	}

	log.Printf("Deleted project %d for user %s", projectID, user.Username)

	// Return success response
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Project deleted successfully",
		})
		return
	}

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// DeployProjectHandler triggers a deployment for a project
func (h *Handler) DeployProjectHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	projectIDStr := chi.URLParam(r, "id")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		log.Printf("Invalid project ID: %v", err)
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// Verify user owns this project
	var ownerID int64
	err = h.DB.QueryRow("SELECT user_id FROM projects WHERE id = ?", projectID).Scan(&ownerID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Project not found", http.StatusNotFound)
		} else {
			log.Printf("Error checking project ownership: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	if ownerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	log.Printf("Deploy project request for project %d from user %s", projectID, user.Username)

	// Trigger deployment (use latest commit)
	deployment, err := h.Deployment.DeployProject(projectID, "")
	if err != nil {
		log.Printf("Error deploying project: %v", err)
		http.Error(w, "Failed to deploy project", http.StatusInternalServerError)
		return
	}

	log.Printf("Started deployment %d for project %d", deployment.ID, projectID)

	// For HTMX requests, return success message
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":       true,
			"message":       "Deployment started",
			"deployment_id": deployment.ID,
		})
		return
	}

	// For regular requests, redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Helper functions

// isValidSubdomain checks if a subdomain is valid (alphanumeric and hyphens only)
func isValidSubdomain(subdomain string) bool {
	if len(subdomain) < 1 || len(subdomain) > 63 {
		return false
	}
	// Must start and end with alphanumeric, can contain hyphens in between
	matched, _ := regexp.MatchString(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, subdomain)
	return matched
}

// subdomainExists checks if a subdomain is already taken
func (h *Handler) subdomainExists(subdomain string) (bool, error) {
	var count int
	err := h.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE subdomain = ?", subdomain).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// generateUniquePort generates a unique port for a project
func (h *Handler) generateUniquePort() (int, error) {
	// Start from port 8081 and find the next available port
	for port := 8081; port <= 9000; port++ {
		var count int
		err := h.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE port = ?", port).Scan(&count)
		if err != nil {
			return 0, err
		}
		if count == 0 {
			return port, nil
		}
	}

	// If no port found in range, generate a random port in higher range
	min := int64(9001)
	max := int64(9999)
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		return 0, err
	}
	return int(n.Int64() + min), nil
}

// generateSubdomain generates a unique subdomain based on project name
func (h *Handler) generateSubdomain(projectName string) (string, error) {
	// Clean the project name
	clean := strings.ToLower(projectName)
	clean = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")

	// Ensure it's not too long
	if len(clean) > 50 {
		clean = clean[:50]
	}

	// Try the clean name first
	if exists, err := h.subdomainExists(clean); err != nil {
		return "", err
	} else if !exists {
		return clean, nil
	}

	// Add random suffix if taken
	for i := 0; i < 10; i++ {
		suffix, err := generateRandomString(4)
		if err != nil {
			return "", err
		}

		candidate := clean + "-" + suffix
		if exists, err := h.subdomainExists(candidate); err != nil {
			return "", err
		} else if !exists {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique subdomain")
}

// generateRandomString generates a random alphanumeric string of given length
func generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}
