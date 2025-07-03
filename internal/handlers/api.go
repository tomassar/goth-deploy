package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// DeploymentsListHandler lists deployments for the authenticated user
func (h *Handler) DeploymentsListHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// TODO: Create deployments list template
	w.Write([]byte(`
		<div class="max-w-4xl mx-auto py-8">
			<h2 class="text-2xl font-bold mb-6">Deployments</h2>
			<p class="text-gray-600">Deployments list coming soon...</p>
			<a href="/dashboard" class="text-purple-600 hover:text-purple-500">← Back to Dashboard</a>
		</div>
	`))
}

// DeploymentDetailsHandler shows details for a specific deployment
func (h *Handler) DeploymentDetailsHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	deploymentID := chi.URLParam(r, "id")
	log.Printf("Deployment details request for deployment %s from user %s", deploymentID, user.Username)

	// TODO: Create deployment details template
	w.Write([]byte(`
		<div class="max-w-4xl mx-auto py-8">
			<h2 class="text-2xl font-bold mb-6">Deployment Details</h2>
			<p class="text-gray-600">Deployment ID: ` + deploymentID + `</p>
			<p class="text-gray-600">Deployment details coming soon...</p>
			<a href="/dashboard" class="text-purple-600 hover:text-purple-500">← Back to Dashboard</a>
		</div>
	`))
}

// GitHubReposHandler returns user's GitHub repositories
func (h *Handler) GitHubReposHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	repos, err := h.GitHub.GetUserRepositories(r.Context(), user.AccessToken)
	if err != nil {
		log.Printf("Error getting GitHub repositories: %v", err)
		http.Error(w, "Failed to fetch repositories", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repos)
}

// GetEnvironmentVariablesHandler returns environment variables for a project
func (h *Handler) GetEnvironmentVariablesHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	projectIDStr := chi.URLParam(r, "projectId")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// TODO: Verify user owns this project

	rows, err := h.DB.Query("SELECT id, key, value FROM environment_variables WHERE project_id = ?", projectID)
	if err != nil {
		log.Printf("Error getting environment variables: %v", err)
		http.Error(w, "Failed to fetch environment variables", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var envVars []map[string]interface{}
	for rows.Next() {
		var id int64
		var key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			continue
		}
		envVars = append(envVars, map[string]interface{}{
			"id":    id,
			"key":   key,
			"value": value,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(envVars)
}

// CreateEnvironmentVariableHandler creates a new environment variable
func (h *Handler) CreateEnvironmentVariableHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	projectIDStr := chi.URLParam(r, "projectId")
	projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	// TODO: Verify user owns this project
	// TODO: Parse request body and create environment variable

	log.Printf("Create environment variable for project %d", projectID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// UpdateEnvironmentVariableHandler updates an environment variable
func (h *Handler) UpdateEnvironmentVariableHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	envVarIDStr := chi.URLParam(r, "id")
	envVarID, err := strconv.ParseInt(envVarIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid environment variable ID", http.StatusBadRequest)
		return
	}

	// TODO: Verify user owns this environment variable
	// TODO: Parse request body and update environment variable

	log.Printf("Update environment variable %d", envVarID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// DeleteEnvironmentVariableHandler deletes an environment variable
func (h *Handler) DeleteEnvironmentVariableHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	envVarIDStr := chi.URLParam(r, "id")
	envVarID, err := strconv.ParseInt(envVarIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid environment variable ID", http.StatusBadRequest)
		return
	}

	// TODO: Verify user owns this environment variable
	// TODO: Delete environment variable

	log.Printf("Delete environment variable %d", envVarID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// BuildLogsHandler returns build logs for a deployment
func (h *Handler) BuildLogsHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	deploymentIDStr := chi.URLParam(r, "id")
	deploymentID, err := strconv.ParseInt(deploymentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid deployment ID", http.StatusBadRequest)
		return
	}

	// TODO: Verify user owns this deployment

	logs, err := h.Deployment.GetDeploymentLogs(deploymentID)
	if err != nil {
		log.Printf("Error getting deployment logs: %v", err)
		http.Error(w, "Failed to fetch deployment logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logs))
}
