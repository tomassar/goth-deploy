package handlers

import (
	"database/sql"
	"log"
	"net/http"

	"goth-deploy/internal/config"
	"goth-deploy/internal/models"
	"goth-deploy/internal/services"
	"goth-deploy/web/templates"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/gorilla/sessions"
)

// Handler holds the dependencies for HTTP handlers
type Handler struct {
	DB         *sql.DB
	Config     *config.Config
	Store      *sessions.CookieStore
	GitHub     *services.GitHubService
	Deployment *services.DeploymentService
	Proxy      *services.ProxyService
}

// New creates a new handler instance
func New(db *sql.DB, cfg *config.Config) *Handler {
	store := sessions.NewCookieStore([]byte(cfg.SessionSecret))
	githubService := services.NewGitHubService(cfg.GitHubClientID, cfg.GitHubClientSecret, cfg.GitHubRedirectURL)
	deploymentService := services.NewDeploymentService(db, cfg)
	proxyService := services.NewProxyService(db, cfg)

	// Wire up the services - proxy service needs reference to deployment service
	proxyService.SetDeploymentService(deploymentService)

	return &Handler{
		DB:         db,
		Config:     cfg,
		Store:      store,
		GitHub:     githubService,
		Deployment: deploymentService,
		Proxy:      proxyService,
	}
}

// Routes sets up the HTTP routes
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// CORS middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"}, // Be more restrictive in production
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes (no auth required)
	r.Get("/", h.HomeHandler)
	r.Get("/health", h.HealthHandler)

	// Auth routes
	r.Route("/auth", func(r chi.Router) {
		r.Get("/github", h.GitHubAuthHandler)
		r.Get("/github/callback", h.GitHubCallbackHandler)
		r.Get("/logout", h.LogoutHandler)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(h.AuthMiddleware)

		// Dashboard
		r.Get("/dashboard", h.DashboardHandler)

		// Projects
		r.Route("/projects", func(r chi.Router) {
			r.Get("/", h.ProjectsListHandler)
			r.Get("/new", h.NewProjectHandler)
			r.Post("/", h.CreateProjectHandler)
			r.Get("/{id}", h.ProjectDetailsHandler)
			r.Put("/{id}", h.UpdateProjectHandler)
			r.Delete("/{id}", h.DeleteProjectHandler)
			r.Post("/{id}/deploy", h.DeployProjectHandler)
		})

		// Deployments
		r.Route("/deployments", func(r chi.Router) {
			r.Get("/", h.DeploymentsListHandler)
			r.Get("/{id}", h.DeploymentDetailsHandler)
		})

		// Environment Variables API
		r.Route("/api/projects/{projectId}/env", func(r chi.Router) {
			r.Get("/", h.GetEnvironmentVariablesHandler)
			r.Post("/", h.CreateEnvironmentVariableHandler)
			r.Put("/{id}", h.UpdateEnvironmentVariableHandler)
			r.Delete("/{id}", h.DeleteEnvironmentVariableHandler)
		})

		// GitHub repos API
		r.Get("/api/github/repos", h.GitHubReposHandler)

		// Build logs
		r.Get("/api/deployments/{id}/logs", h.BuildLogsHandler)
	})

	return r
}

// HomeHandler serves the landing page for unauthenticated users
func (h *Handler) HomeHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is authenticated
	user := h.getCurrentUser(r)
	if user != nil {
		// Redirect to dashboard if already logged in
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Serve landing page
	component := templates.Home()
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering home template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// DashboardHandler serves the main dashboard
func (h *Handler) DashboardHandler(w http.ResponseWriter, r *http.Request) {
	user := h.getCurrentUser(r)
	if user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Get dashboard data
	data, err := h.getDashboardData(user.ID)
	if err != nil {
		log.Printf("Error getting dashboard data: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	component := templates.Dashboard(user, data)
	if err := component.Render(r.Context(), w); err != nil {
		log.Printf("Error rendering dashboard template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// HealthHandler provides a health check endpoint
func (h *Handler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// getDashboardData retrieves data for the dashboard
func (h *Handler) getDashboardData(userID int64) (templates.DashboardData, error) {
	data := templates.DashboardData{}

	// Get projects count
	err := h.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE user_id = ?", userID).Scan(&data.TotalProjects)
	if err != nil {
		return data, err
	}

	// Get active projects count
	err = h.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE user_id = ? AND status = 'active'", userID).Scan(&data.ActiveProjects)
	if err != nil {
		return data, err
	}

	// Get deployments today count
	err = h.DB.QueryRow(`
		SELECT COUNT(*) FROM deployments d 
		JOIN projects p ON d.project_id = p.id 
		WHERE p.user_id = ? AND DATE(d.created_at) = DATE('now')
	`, userID).Scan(&data.DeployedToday)
	if err != nil {
		return data, err
	}

	// Calculate success rate
	var totalDeployments, successfulDeployments int
	err = h.DB.QueryRow(`
		SELECT COUNT(*) FROM deployments d 
		JOIN projects p ON d.project_id = p.id 
		WHERE p.user_id = ?
	`, userID).Scan(&totalDeployments)
	if err != nil {
		return data, err
	}

	if totalDeployments > 0 {
		err = h.DB.QueryRow(`
			SELECT COUNT(*) FROM deployments d 
			JOIN projects p ON d.project_id = p.id 
			WHERE p.user_id = ? AND d.status = 'success'
		`, userID).Scan(&successfulDeployments)
		if err != nil {
			return data, err
		}
		data.SuccessRate = float64(successfulDeployments) / float64(totalDeployments) * 100
	} else {
		data.SuccessRate = 98.5 // Default value when no deployments yet
	}

	// Get recent projects
	rows, err := h.DB.Query(`
		SELECT id, name, repo_url, branch, subdomain, status, last_deploy 
		FROM projects 
		WHERE user_id = ? 
		ORDER BY created_at DESC 
		LIMIT 10
	`, userID)
	if err != nil {
		return data, err
	}
	defer rows.Close()

	for rows.Next() {
		var project models.Project
		err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.RepoURL,
			&project.Branch,
			&project.Subdomain,
			&project.Status,
			&project.LastDeploy,
		)
		if err != nil {
			return data, err
		}
		data.Projects = append(data.Projects, project)
	}

	return data, nil
}

// getCurrentUser retrieves the current user from session
func (h *Handler) getCurrentUser(r *http.Request) *models.User {
	session, err := h.Store.Get(r, "goth-session")
	if err != nil {
		return nil
	}

	userID, ok := session.Values["user_id"].(int64)
	if !ok {
		return nil
	}

	var user models.User
	err = h.DB.QueryRow(`
		SELECT id, github_id, username, email, avatar_url, access_token, created_at, updated_at 
		FROM users WHERE id = ?
	`, userID).Scan(
		&user.ID,
		&user.GitHubID,
		&user.Username,
		&user.Email,
		&user.AvatarURL,
		&user.AccessToken,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil
	}

	return &user
}
