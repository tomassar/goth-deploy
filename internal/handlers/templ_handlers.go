package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"deployer/internal/config"
	"deployer/internal/models"
	"deployer/internal/services"
	"deployer/web/templates"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type TemplHandler struct {
	db                *sql.DB
	githubService     *services.GitHubService
	deploymentService *services.DeploymentService
	config            *config.Config
}

func NewTemplHandler(db *sql.DB, githubService *services.GitHubService, deploymentService *services.DeploymentService, cfg *config.Config) *TemplHandler {
	return &TemplHandler{
		db:                db,
		githubService:     githubService,
		deploymentService: deploymentService,
		config:            cfg,
	}
}

// Session management helpers (same as before)
func (h *TemplHandler) setSession(w http.ResponseWriter, key, value string) {
	cookie := &http.Cookie{
		Name:     key,
		Value:    value,
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   h.config.Environment == "production",
	}
	http.SetCookie(w, cookie)
}

func (h *TemplHandler) getSession(r *http.Request, key string) string {
	cookie, err := r.Cookie(key)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *TemplHandler) clearSession(w http.ResponseWriter, key string) {
	cookie := &http.Cookie{
		Name:     key,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
}

func (h *TemplHandler) getCurrentUser(r *http.Request) (*models.User, error) {
	userID := h.getSession(r, "user_id")
	if userID == "" {
		return nil, fmt.Errorf("no user session")
	}

	id, err := strconv.Atoi(userID)
	if err != nil {
		return nil, err
	}

	var user models.User
	err = h.db.QueryRow(`
		SELECT id, github_id, username, email, avatar_url, access_token, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.GitHubID, &user.Username, &user.Email, &user.AvatarURL, &user.AccessToken, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (h *TemplHandler) Home(w http.ResponseWriter, r *http.Request) {
	user, _ := h.getCurrentUser(r)
	if user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates.Home().Render(context.Background(), w)
}

func (h *TemplHandler) GitHubAuth(w http.ResponseWriter, r *http.Request) {
	state := fmt.Sprintf("%d", time.Now().Unix())
	h.setSession(w, "oauth_state", state)

	redirectURL := fmt.Sprintf("http://%s/auth/github/callback", r.Host)
	authURL := h.githubService.GetAuthURL(state, redirectURL)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *TemplHandler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	sessionState := h.getSession(r, "oauth_state")

	if state != sessionState {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code not found", http.StatusBadRequest)
		return
	}

	redirectURL := fmt.Sprintf("http://%s/auth/github/callback", r.Host)
	token, err := h.githubService.ExchangeCode(code, redirectURL)
	if err != nil {
		http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
		return
	}

	githubUser, err := h.githubService.GetUser(token)
	if err != nil {
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	emails, err := h.githubService.GetUserEmails(token)
	if err != nil {
		http.Error(w, "Failed to get user emails", http.StatusInternalServerError)
		return
	}

	var primaryEmail string
	for _, email := range emails {
		if email.GetPrimary() {
			primaryEmail = email.GetEmail()
			break
		}
	}

	// Save or update user
	user, err := h.saveUser(githubUser, primaryEmail, token.AccessToken)
	if err != nil {
		http.Error(w, "Failed to save user", http.StatusInternalServerError)
		return
	}

	h.setSession(w, "user_id", strconv.Itoa(user.ID))
	h.clearSession(w, "oauth_state")

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *TemplHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	projects, err := h.getUserProjects(user.ID)
	if err != nil {
		http.Error(w, "Failed to get projects", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	templates.Dashboard(user, projects).Render(context.Background(), w)
}

func (h *TemplHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get GitHub repositories
	token := &oauth2.Token{AccessToken: user.AccessToken}
	repos, err := h.githubService.ListRepositories(token)
	if err != nil {
		http.Error(w, "Failed to get repositories", http.StatusInternalServerError)
		return
	}

	// Get existing projects
	existingProjects, err := h.getUserProjects(user.ID)
	if err != nil {
		http.Error(w, "Failed to get existing projects", http.StatusInternalServerError)
		return
	}

	// Filter out already deployed repositories
	existingRepos := make(map[string]bool)
	for _, project := range existingProjects {
		existingRepos[project.Repository] = true
	}

	var availableRepos []interface{}
	for _, repo := range repos {
		if !existingRepos[repo.GetCloneURL()] {
			availableRepos = append(availableRepos, map[string]interface{}{
				"name":        repo.GetName(),
				"description": repo.GetDescription(),
				"clone_url":   repo.GetCloneURL(),
				"updated_at":  repo.GetUpdatedAt().Format("Jan 2, 2006"),
			})
		}
	}

	w.Header().Set("Content-Type", "text/html")
	templates.ProjectList(availableRepos).Render(context.Background(), w)
}

func (h *TemplHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	repository := r.FormValue("repository")
	branch := r.FormValue("branch")

	if branch == "" {
		branch = "main"
	}

	subdomain := h.deploymentService.GenerateSubdomain(name)

	// Create project
	result, err := h.db.Exec(`
		INSERT INTO projects (user_id, name, repository, branch, subdomain, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, name, repository, branch, subdomain, models.ProjectStatusIdle, time.Now(), time.Now())

	if err != nil {
		http.Error(w, "Failed to create project", http.StatusInternalServerError)
		return
	}

	projectID, _ := result.LastInsertId()

	// Return HTMX response
	w.Header().Set("HX-Trigger", "projectCreated")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"project_id": projectID,
		"message":    "Project created successfully",
	})
}

func (h *TemplHandler) DeployProject(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.getProject(projectID, user.ID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Update project status
	_, err = h.db.Exec(`
		UPDATE projects SET status = ?, updated_at = ? WHERE id = ?
	`, models.ProjectStatusBuilding, time.Now(), project.ID)
	if err != nil {
		http.Error(w, "Failed to update project status", http.StatusInternalServerError)
		return
	}

	// Start deployment in background
	go func() {
		ctx := context.Background()
		err := h.deploymentService.Deploy(ctx, project, "")

		status := models.ProjectStatusActive
		if err != nil {
			status = models.ProjectStatusFailed
		}

		// Update project status
		h.db.Exec(`
			UPDATE projects SET status = ?, last_deploy_at = ?, updated_at = ? WHERE id = ?
		`, status, time.Now(), time.Now(), project.ID)
	}()

	// Return HTMX response
	w.Header().Set("HX-Trigger", "deploymentStarted")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Deployment started",
	})
}

// Helper functions
func (h *TemplHandler) saveUser(githubUser *github.User, email, accessToken string) (*models.User, error) {
	user := &models.User{
		GitHubID:    int(githubUser.GetID()),
		Username:    githubUser.GetLogin(),
		Email:       email,
		AvatarURL:   githubUser.GetAvatarURL(),
		AccessToken: accessToken,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	result, err := h.db.Exec(`
		INSERT OR REPLACE INTO users (github_id, username, email, avatar_url, access_token, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, user.GitHubID, user.Username, user.Email, user.AvatarURL, user.AccessToken, user.CreatedAt, user.UpdatedAt)

	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	user.ID = int(id)

	return user, nil
}

func (h *TemplHandler) getUserProjects(userID int) ([]models.Project, error) {
	rows, err := h.db.Query(`
		SELECT id, user_id, name, repository, branch, subdomain, status, last_deploy_at, created_at, updated_at
		FROM projects WHERE user_id = ? ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var project models.Project
		err := rows.Scan(&project.ID, &project.UserID, &project.Name, &project.Repository,
			&project.Branch, &project.Subdomain, &project.Status, &project.LastDeployAt,
			&project.CreatedAt, &project.UpdatedAt)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	return projects, nil
}

func (h *TemplHandler) getProject(projectID string, userID int) (*models.Project, error) {
	var project models.Project
	err := h.db.QueryRow(`
		SELECT id, user_id, name, repository, branch, subdomain, status, last_deploy_at, created_at, updated_at
		FROM projects WHERE id = ? AND user_id = ?
	`, projectID, userID).Scan(&project.ID, &project.UserID, &project.Name, &project.Repository,
		&project.Branch, &project.Subdomain, &project.Status, &project.LastDeployAt,
		&project.CreatedAt, &project.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return &project, nil
}

func (h *TemplHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.getProject(projectID, user.ID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// For now, return a simple JSON response
	// In a full implementation, you'd have a project detail template
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"project": project,
		"url":     h.deploymentService.GetProjectURL(project.Subdomain),
	})
}

func (h *TemplHandler) GetDeploymentLogs(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.getProject(projectID, user.ID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	logs, err := h.deploymentService.GetDeploymentLogs(project.Subdomain)
	if err != nil {
		http.Error(w, "Failed to get logs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs": logs,
	})
}

func (h *TemplHandler) StopProject(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.getProject(projectID, user.ID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Stop the service
	err = h.deploymentService.StopService(project.Subdomain)
	if err != nil {
		http.Error(w, "Failed to stop deployment", http.StatusInternalServerError)
		return
	}

	// Update project status to idle
	_, err = h.db.Exec(`
		UPDATE projects SET status = ?, updated_at = ? WHERE id = ?
	`, models.ProjectStatusIdle, time.Now(), project.ID)
	if err != nil {
		http.Error(w, "Failed to update project status", http.StatusInternalServerError)
		return
	}

	// Return HTMX response
	w.Header().Set("HX-Trigger", "projectStopped")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Deployment stopped successfully",
	})
}

func (h *TemplHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.getProject(projectID, user.ID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}

	// Delete the deployment (this stops the service and cleans up files)
	err = h.deploymentService.DeleteDeployment(project.Subdomain)
	if err != nil {
		http.Error(w, "Failed to delete deployment", http.StatusInternalServerError)
		return
	}

	// Delete from database
	_, err = h.db.Exec(`DELETE FROM projects WHERE id = ? AND user_id = ?`, project.ID, user.ID)
	if err != nil {
		http.Error(w, "Failed to delete project from database", http.StatusInternalServerError)
		return
	}

	// Return HTMX response
	w.Header().Set("HX-Trigger", "projectDeleted")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Project deleted successfully",
	})
}

func (h *TemplHandler) StopAllProjects(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Stop all deployments
	err = h.deploymentService.StopAllDeployments()
	if err != nil {
		http.Error(w, "Failed to stop all deployments", http.StatusInternalServerError)
		return
	}

	// Update all user's projects to idle status
	_, err = h.db.Exec(`
		UPDATE projects SET status = ?, updated_at = ? WHERE user_id = ? AND status = ?
	`, models.ProjectStatusIdle, time.Now(), user.ID, models.ProjectStatusActive)
	if err != nil {
		http.Error(w, "Failed to update project statuses", http.StatusInternalServerError)
		return
	}

	// Return HTMX response
	w.Header().Set("HX-Trigger", "allProjectsStopped")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "All deployments stopped successfully",
	})
}

func (h *TemplHandler) GetActiveDeployments(w http.ResponseWriter, r *http.Request) {
	user, err := h.getCurrentUser(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get active deployments from service
	activeDeployments := h.deploymentService.GetActiveDeployments()

	// Get user's projects
	projects, err := h.getUserProjects(user.ID)
	if err != nil {
		http.Error(w, "Failed to get projects", http.StatusInternalServerError)
		return
	}

	// Filter to only user's active projects
	var userActiveDeployments []string
	for _, project := range projects {
		for _, active := range activeDeployments {
			if project.Subdomain == active && project.Status == models.ProjectStatusActive {
				userActiveDeployments = append(userActiveDeployments, active)
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_deployments": userActiveDeployments,
		"count":              len(userActiveDeployments),
	})
}
