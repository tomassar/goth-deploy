package services

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"deployer/internal/models"
)

type DeploymentService struct {
	deploymentPath string
	baseDomain     string
	proxyService   *ProxyService
	portTracker    map[string]int // subdomain -> port
	portMux        sync.RWMutex
}

func NewDeploymentService(deploymentPath, baseDomain string, proxyService *ProxyService) *DeploymentService {
	return &DeploymentService{
		deploymentPath: deploymentPath,
		baseDomain:     baseDomain,
		proxyService:   proxyService,
		portTracker:    make(map[string]int),
	}
}

func (s *DeploymentService) Deploy(ctx context.Context, project *models.Project, commitSHA string) error {
	projectPath := filepath.Join(s.deploymentPath, project.Subdomain)

	// Create deployment directory if it doesn't exist
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return fmt.Errorf("failed to create deployment directory: %w", err)
	}

	// Clone or pull repository
	if err := s.cloneOrPullRepo(ctx, project.Repository, project.Branch, projectPath); err != nil {
		return fmt.Errorf("failed to clone/pull repository: %w", err)
	}

	// Build the project
	if err := s.buildProject(ctx, projectPath); err != nil {
		return fmt.Errorf("failed to build project: %w", err)
	}

	// Start the service
	if err := s.startService(ctx, project.Subdomain, projectPath); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

func (s *DeploymentService) cloneOrPullRepo(ctx context.Context, repoURL, branch, projectPath string) error {
	gitDir := filepath.Join(projectPath, ".git")

	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Clone repository
		cmd := exec.CommandContext(ctx, "git", "clone", "-b", branch, repoURL, projectPath)
		return cmd.Run()
	} else {
		// Pull latest changes
		cmd := exec.CommandContext(ctx, "git", "-C", projectPath, "fetch", "origin", branch)
		if err := cmd.Run(); err != nil {
			return err
		}

		cmd = exec.CommandContext(ctx, "git", "-C", projectPath, "reset", "--hard", "origin/"+branch)
		return cmd.Run()
	}
}

func (s *DeploymentService) buildProject(ctx context.Context, projectPath string) error {
	// Check if go.mod exists
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err != nil {
		return fmt.Errorf("go.mod not found in project")
	}

	// Download dependencies
	cmd := exec.CommandContext(ctx, "go", "mod", "download")
	cmd.Dir = projectPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download dependencies: %w", err)
	}

	// Check if templ files exist and generate them
	templFiles, _ := filepath.Glob(filepath.Join(projectPath, "**/*.templ"))
	if len(templFiles) > 0 {
		cmd = exec.CommandContext(ctx, "templ", "generate")
		cmd.Dir = projectPath
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to generate templ files: %w", err)
		}
	}

	// Build the application
	cmd = exec.CommandContext(ctx, "go", "build", "-o", "app", "./cmd/server")
	cmd.Dir = projectPath
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to build application: %w", err)
	}

	return nil
}

func (s *DeploymentService) startService(ctx context.Context, subdomain, projectPath string) error {
	// Stop existing service if running
	s.StopService(subdomain)

	// Find available port
	port, err := s.findAvailablePort()
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}

	// Start new service
	cmd := exec.CommandContext(ctx, "./app")
	cmd.Dir = projectPath
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("SUBDOMAIN=%s", subdomain),
	)

	// Create logs directory
	logsDir := filepath.Join(s.deploymentPath, "logs")
	os.MkdirAll(logsDir, 0755)

	// Create log file
	logFile, err := os.Create(filepath.Join(logsDir, subdomain+".log"))
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Track the port for this subdomain
	s.portMux.Lock()
	s.portTracker[subdomain] = port
	s.portMux.Unlock()

	// Add route to proxy service
	targetURL := fmt.Sprintf("http://localhost:%d", port)
	s.proxyService.AddRoute(subdomain, targetURL)

	return nil
}

// findAvailablePort finds an available port starting from 3000
func (s *DeploymentService) findAvailablePort() (int, error) {
	for port := 3000; port < 4000; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports found in range 3000-4000")
}

func (s *DeploymentService) StopService(subdomain string) error {
	// Remove from proxy service
	s.proxyService.RemoveRoute(subdomain)

	// Remove from port tracker
	s.portMux.Lock()
	delete(s.portTracker, subdomain)
	s.portMux.Unlock()

	// This is a simplified approach - in production you'd want proper process management
	cmd := exec.Command("pkill", "-f", subdomain)
	cmd.Run() // Ignore errors as the process might not be running
	return nil
}

func (s *DeploymentService) GetDeploymentLogs(subdomain string) ([]string, error) {
	logFile := filepath.Join(s.deploymentPath, "logs", subdomain+".log")

	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{"No logs available yet."}, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	// Get last 100 lines
	allLines := make([]string, 0)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	start := len(allLines) - 100
	if start < 0 {
		start = 0
	}

	lines = allLines[start:]

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func (s *DeploymentService) GenerateSubdomain(projectName string) string {
	// Simple subdomain generation - in production you'd want better logic
	subdomain := strings.ToLower(projectName)
	subdomain = strings.ReplaceAll(subdomain, " ", "-")
	subdomain = strings.ReplaceAll(subdomain, "_", "-")

	// Add timestamp to ensure uniqueness
	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%d", subdomain, timestamp)
}

func (s *DeploymentService) GetProjectURL(subdomain string) string {
	return fmt.Sprintf("http://%s.%s", subdomain, s.baseDomain)
}

// RestartActiveDeployments restarts all active deployments on server startup
func (s *DeploymentService) RestartActiveDeployments(db *sql.DB) error {
	// Query for active projects
	rows, err := db.Query(`
		SELECT id, name, subdomain, repository, branch FROM projects WHERE status = ?
	`, "active")
	if err != nil {
		return fmt.Errorf("failed to query active projects: %w", err)
	}
	defer rows.Close()

	var restarted, failed, rebuilt int

	for rows.Next() {
		var id int
		var name, subdomain, repository, branch string
		if err := rows.Scan(&id, &name, &subdomain, &repository, &branch); err != nil {
			fmt.Printf("Failed to scan project row: %v\n", err)
			failed++
			continue
		}

		fmt.Printf("Attempting to restart project: %s (%s)\n", name, subdomain)

		// Check if deployment directory exists
		projectPath := filepath.Join(s.deploymentPath, subdomain)
		if _, err := os.Stat(projectPath); os.IsNotExist(err) {
			fmt.Printf("Project directory missing for %s, marking as failed\n", subdomain)
			s.updateProjectStatus(db, id, "failed")
			failed++
			continue
		}

		// Check if built app exists
		appPath := filepath.Join(projectPath, "app")
		if _, err := os.Stat(appPath); os.IsNotExist(err) {
			fmt.Printf("Built app missing for %s, attempting rebuild...\n", subdomain)

			// Try to rebuild the project
			if err := s.buildProject(context.Background(), projectPath); err != nil {
				fmt.Printf("Failed to rebuild %s: %v\n", subdomain, err)
				s.updateProjectStatus(db, id, "failed")
				failed++
				continue
			}

			// Check again if app was built successfully
			if _, err := os.Stat(appPath); os.IsNotExist(err) {
				fmt.Printf("Rebuild failed for %s - app binary still missing\n", subdomain)
				s.updateProjectStatus(db, id, "failed")
				failed++
				continue
			}

			fmt.Printf("Successfully rebuilt %s\n", subdomain)
			rebuilt++
		}

		// Start the service
		if err := s.startService(context.Background(), subdomain, projectPath); err != nil {
			fmt.Printf("Failed to start service for %s: %v\n", subdomain, err)
			s.updateProjectStatus(db, id, "failed")
			failed++
			continue
		}

		fmt.Printf("Successfully restarted %s\n", subdomain)
		restarted++
	}

	// Log summary
	if restarted > 0 {
		fmt.Printf("✓ Successfully restarted %d active deployments\n", restarted)
	}
	if rebuilt > 0 {
		fmt.Printf("🔨 Rebuilt %d missing app binaries\n", rebuilt)
	}
	if failed > 0 {
		fmt.Printf("❌ Failed to restart %d deployments\n", failed)
		return fmt.Errorf("restarted %d deployments, rebuilt %d apps, failed to restart %d deployments", restarted, rebuilt, failed)
	}

	return nil
}

// Helper function to update project status
func (s *DeploymentService) updateProjectStatus(db *sql.DB, projectID int, status string) {
	_, err := db.Exec(`UPDATE projects SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, status, projectID)
	if err != nil {
		fmt.Printf("Failed to update project %d status to %s: %v\n", projectID, status, err)
	}
}
