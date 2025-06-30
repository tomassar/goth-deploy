package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"deployer/internal/config"
	"deployer/internal/models"
	"deployer/internal/services"

	"github.com/go-chi/chi/v5"
	"golang.org/x/oauth2"
)

type Handler struct {
	db                *sql.DB
	githubService     *services.GitHubService
	deploymentService *services.DeploymentService
	config            *config.Config
}

func New(db *sql.DB, githubService *services.GitHubService, deploymentService *services.DeploymentService, cfg *config.Config) *Handler {
	return &Handler{
		db:                db,
		githubService:     githubService,
		deploymentService: deploymentService,
		config:            cfg,
	}
}

// Session management helpers
func (h *Handler) setSession(w http.ResponseWriter, key, value string) {
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

func (h *Handler) getSession(r *http.Request, key string) string {
	cookie, err := r.Cookie(key)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (h *Handler) clearSession(w http.ResponseWriter, key string) {
	cookie := &http.Cookie{
		Name:     key,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
}

func (h *Handler) getCurrentUser(r *http.Request) (*models.User, error) {
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

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	user, _ := h.getCurrentUser(r)
	if user != nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	data := struct {
		Title string
	}{
		Title: "GoTH Deployer - Deploy your GoTH apps instantly",
	}

	h.renderTemplate(w, "home", data)
}

func (h *Handler) GitHubAuth(w http.ResponseWriter, r *http.Request) {
	state := fmt.Sprintf("%d", time.Now().Unix())
	h.setSession(w, "oauth_state", state)

	redirectURL := fmt.Sprintf("http://%s/auth/github/callback", r.Host)
	authURL := h.githubService.GetAuthURL(state, redirectURL)

	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (h *Handler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
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

	data := struct {
		Title    string
		User     *models.User
		Projects []models.Project
	}{
		Title:    "Dashboard",
		User:     user,
		Projects: projects,
	}

	h.renderTemplate(w, "dashboard", data)
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
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
				"updated_at":  repo.GetUpdatedAt(),
			})
		}
	}

	data := struct {
		Repositories []interface{}
	}{
		Repositories: availableRepos,
	}

	h.renderTemplate(w, "project-list", data)
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
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

	data := struct {
		Title   string
		User    *models.User
		Project *models.Project
		URL     string
	}{
		Title:   project.Name,
		User:    user,
		Project: project,
		URL:     h.deploymentService.GetProjectURL(project.Subdomain),
	}

	h.renderTemplate(w, "project-detail", data)
}

func (h *Handler) DeployProject(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) GetDeploymentLogs(w http.ResponseWriter, r *http.Request) {
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

	data := struct {
		Logs []string
	}{
		Logs: logs,
	}

	h.renderTemplate(w, "deployment-logs", data)
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
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
	h.deploymentService.StopService(project.Subdomain)

	// Delete from database
	_, err = h.db.Exec(`DELETE FROM projects WHERE id = ? AND user_id = ?`, project.ID, user.ID)
	if err != nil {
		http.Error(w, "Failed to delete project", http.StatusInternalServerError)
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

// Helper functions
func (h *Handler) saveUser(githubUser interface{}, email, accessToken string) (*models.User, error) {
	// This is a simplified implementation - you'd want to use proper GitHub user type
	user := &models.User{
		GitHubID:    1, // Would extract from githubUser
		Username:    "github-user",
		Email:       email,
		AvatarURL:   "",
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

func (h *Handler) getUserProjects(userID int) ([]models.Project, error) {
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

func (h *Handler) getProject(projectID string, userID int) (*models.Project, error) {
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

func (h *Handler) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	// This is a simplified template rendering - you'd want to use proper template files
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>{{.Title}}</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <script src="https://cdn.tailwindcss.com"></script>
</head>
<body class="bg-gray-50">
    <!-- Content will be rendered here -->
    <div class="min-h-screen">
        <h1 class="text-3xl font-bold text-center py-8">{{.Title}}</h1>
        <!-- Template-specific content would go here -->
    </div>
</body>
</html>`

	t, err := template.New(templateName).Parse(tmpl)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	t.Execute(w, data)
}
