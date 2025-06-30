package services

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"deployer/internal/models"
)

type DeploymentService struct {
	deploymentPath string
	baseDomain     string
	proxyService   *ProxyService
	portTracker    map[string]int         // subdomain -> port
	processTracker map[string]*os.Process // subdomain -> process
	portMux        sync.RWMutex
	processMux     sync.RWMutex
}

func NewDeploymentService(deploymentPath, baseDomain string, proxyService *ProxyService) *DeploymentService {
	return &DeploymentService{
		deploymentPath: deploymentPath,
		baseDomain:     baseDomain,
		proxyService:   proxyService,
		portTracker:    make(map[string]int),
		processTracker: make(map[string]*os.Process),
	}
}

func (s *DeploymentService) Deploy(ctx context.Context, project *models.Project, commitSHA string) error {
	projectPath := filepath.Join(s.deploymentPath, project.Subdomain)

	// Create deployment directory if it doesn't exist
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		buildError := fmt.Sprintf("Failed to create deployment directory: %v", err)
		s.saveBuildLogs(project.ID, buildError)
		return fmt.Errorf(buildError)
	}

	// Clone or pull repository
	if err := s.cloneOrPullRepo(ctx, project.Repository, project.Branch, projectPath); err != nil {
		buildError := fmt.Sprintf("Failed to clone/pull repository: %v", err)
		s.saveBuildLogs(project.ID, buildError)
		return fmt.Errorf(buildError)
	}

	// Build the project and capture logs
	buildLogs, err := s.buildProjectWithLogs(ctx, projectPath)
	if err != nil {
		errorMsg := fmt.Sprintf("Build failed: %v", err)
		s.saveBuildLogs(project.ID, fmt.Sprintf("%s\n\nBuild Output:\n%s", errorMsg, buildLogs))
		return fmt.Errorf(errorMsg)
	}

	// Save successful build logs
	s.saveBuildLogs(project.ID, fmt.Sprintf("Build completed successfully at %s\n\nBuild Output:\n%s", time.Now().Format("2006-01-02 15:04:05"), buildLogs))

	// Start the service
	if err := s.startService(ctx, project.Subdomain, projectPath); err != nil {
		errorMsg := fmt.Sprintf("Failed to start service: %v", err)
		s.saveBuildLogs(project.ID, fmt.Sprintf("Service start failed: %v\n\nPrevious build was successful.", err))
		return fmt.Errorf(errorMsg)
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

func (s *DeploymentService) buildProjectWithLogs(ctx context.Context, projectPath string) (string, error) {
	var logBuffer bytes.Buffer

	// Check if go.mod exists
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err != nil {
		return "", fmt.Errorf("go.mod not found in project")
	}

	logBuffer.WriteString("=== Starting Build Process ===\n")
	logBuffer.WriteString(fmt.Sprintf("Build started at: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	logBuffer.WriteString(fmt.Sprintf("Project path: %s\n\n", projectPath))

	// Download dependencies
	logBuffer.WriteString("=== Downloading Dependencies ===\n")
	cmd := exec.CommandContext(ctx, "go", "mod", "download")
	cmd.Dir = projectPath

	output, err := cmd.CombinedOutput()
	logBuffer.Write(output)
	logBuffer.WriteString("\n")

	if err != nil {
		logBuffer.WriteString(fmt.Sprintf("ERROR: Failed to download dependencies: %v\n", err))
		return logBuffer.String(), fmt.Errorf("failed to download dependencies: %w", err)
	}
	logBuffer.WriteString("Dependencies downloaded successfully\n\n")

	// Check if templ files exist and generate them
	templFiles, _ := filepath.Glob(filepath.Join(projectPath, "**/*.templ"))
	if len(templFiles) > 0 {
		logBuffer.WriteString("=== Generating Templ Files ===\n")
		logBuffer.WriteString(fmt.Sprintf("Found %d templ files\n", len(templFiles)))

		cmd = exec.CommandContext(ctx, "templ", "generate")
		cmd.Dir = projectPath

		output, err := cmd.CombinedOutput()
		logBuffer.Write(output)
		logBuffer.WriteString("\n")

		if err != nil {
			logBuffer.WriteString(fmt.Sprintf("ERROR: Failed to generate templ files: %v\n", err))
			return logBuffer.String(), fmt.Errorf("failed to generate templ files: %w", err)
		}
		logBuffer.WriteString("Templ files generated successfully\n\n")
	}

	// Build the application
	logBuffer.WriteString("=== Building Application ===\n")
	cmd = exec.CommandContext(ctx, "go", "build", "-v", "-o", "app", "./cmd/server")
	cmd.Dir = projectPath

	output, err = cmd.CombinedOutput()
	logBuffer.Write(output)
	logBuffer.WriteString("\n")

	if err != nil {
		logBuffer.WriteString(fmt.Sprintf("ERROR: Failed to build application: %v\n", err))
		return logBuffer.String(), fmt.Errorf("failed to build application: %w", err)
	}

	logBuffer.WriteString("Application built successfully\n")
	logBuffer.WriteString(fmt.Sprintf("Build completed at: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	logBuffer.WriteString("=== Build Process Complete ===\n")

	return logBuffer.String(), nil
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

	// Track the port and process for this subdomain
	s.portMux.Lock()
	s.portTracker[subdomain] = port
	s.portMux.Unlock()

	s.processMux.Lock()
	s.processTracker[subdomain] = cmd.Process
	s.processMux.Unlock()

	// Add route to proxy service
	targetURL := fmt.Sprintf("http://localhost:%d", port)
	s.proxyService.AddRoute(subdomain, targetURL)

	fmt.Printf("Started service for %s on port %d (PID: %d)\n", subdomain, port, cmd.Process.Pid)
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

// StopService stops a running deployment service
func (s *DeploymentService) StopService(subdomain string) error {
	fmt.Printf("Stopping service for %s...\n", subdomain)

	// Remove from proxy service
	s.proxyService.RemoveRoute(subdomain)

	// Get and remove process
	s.processMux.Lock()
	process, exists := s.processTracker[subdomain]
	if exists {
		delete(s.processTracker, subdomain)
	}
	s.processMux.Unlock()

	// Remove from port tracker
	s.portMux.Lock()
	port, portExists := s.portTracker[subdomain]
	delete(s.portTracker, subdomain)
	s.portMux.Unlock()

	// Kill the specific process if we have it
	if exists && process != nil {
		fmt.Printf("Killing process PID %d for %s\n", process.Pid, subdomain)
		if err := process.Kill(); err != nil {
			fmt.Printf("Failed to kill process %d: %v\n", process.Pid, err)
		}
	}

	// Fallback: kill by port if we know it
	if portExists {
		s.killProcessByPort(port)
	}

	// Additional fallback: kill by subdomain name pattern
	cmd := exec.Command("pkill", "-f", subdomain)
	if err := cmd.Run(); err != nil {
		// Ignore errors as the process might not be running
		fmt.Printf("pkill for %s: %v (this is normal if process wasn't running)\n", subdomain, err)
	}

	fmt.Printf("Service stopped for %s\n", subdomain)
	return nil
}

// DeleteDeployment completely removes a deployment including files
func (s *DeploymentService) DeleteDeployment(subdomain string) error {
	fmt.Printf("Deleting deployment for %s...\n", subdomain)

	// First stop the service
	if err := s.StopService(subdomain); err != nil {
		fmt.Printf("Warning: Failed to stop service for %s: %v\n", subdomain, err)
	}

	// Remove deployment directory
	projectPath := filepath.Join(s.deploymentPath, subdomain)
	if _, err := os.Stat(projectPath); err == nil {
		fmt.Printf("Removing deployment directory: %s\n", projectPath)
		if err := os.RemoveAll(projectPath); err != nil {
			return fmt.Errorf("failed to remove deployment directory: %w", err)
		}
	}

	// Remove log file
	logFile := filepath.Join(s.deploymentPath, "logs", subdomain+".log")
	if _, err := os.Stat(logFile); err == nil {
		fmt.Printf("Removing log file: %s\n", logFile)
		if err := os.Remove(logFile); err != nil {
			fmt.Printf("Warning: Failed to remove log file: %v\n", err)
		}
	}

	fmt.Printf("Deployment deleted for %s\n", subdomain)
	return nil
}

// killProcessByPort kills a process running on a specific port
func (s *DeploymentService) killProcessByPort(port int) {
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
	output, err := cmd.Output()
	if err != nil {
		return // No process found on this port
	}

	pidStr := strings.TrimSpace(string(output))
	if pidStr == "" {
		return
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return
	}

	fmt.Printf("Killing process PID %d on port %d\n", pid, port)
	killCmd := exec.Command("kill", "-9", strconv.Itoa(pid))
	killCmd.Run() // Ignore errors
}

// GetActiveDeployments returns a list of currently active deployments
func (s *DeploymentService) GetActiveDeployments() []string {
	s.portMux.RLock()
	defer s.portMux.RUnlock()

	var active []string
	for subdomain := range s.portTracker {
		active = append(active, subdomain)
	}
	return active
}

// StopAllDeployments stops all running deployments
func (s *DeploymentService) StopAllDeployments() error {
	active := s.GetActiveDeployments()

	fmt.Printf("Stopping %d active deployments...\n", len(active))

	for _, subdomain := range active {
		if err := s.StopService(subdomain); err != nil {
			fmt.Printf("Failed to stop %s: %v\n", subdomain, err)
		}
	}

	fmt.Printf("All deployments stopped\n")
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
			if _, err := s.buildProjectWithLogs(context.Background(), projectPath); err != nil {
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

// saveBuildLogs saves build logs to the database
func (s *DeploymentService) saveBuildLogs(projectID int, logs string) {
	// For now, just print to console since we don't have database access in this service
	// The actual saving will be handled by the handlers using SaveBuildLogsToDatabase
	fmt.Printf("Build logs for project %d:\n%s\n", projectID, logs)
}

// SaveBuildLogsToDatabase saves build logs to the database (called from handlers)
func (s *DeploymentService) SaveBuildLogsToDatabase(db *sql.DB, projectID int, logs string) error {
	_, err := db.Exec(`
		UPDATE projects SET build_logs = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, logs, projectID)
	return err
}

// GetProjectDeployments gets deployment history for a project
func (s *DeploymentService) GetProjectDeployments(db *sql.DB, projectID int) ([]models.Deployment, error) {
	rows, err := db.Query(`
		SELECT id, project_id, status, commit_sha, logs, error_message, started_at, ended_at
		FROM deployments WHERE project_id = ? ORDER BY started_at DESC LIMIT 10
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []models.Deployment
	for rows.Next() {
		var deployment models.Deployment
		err := rows.Scan(&deployment.ID, &deployment.ProjectID, &deployment.Status,
			&deployment.CommitSHA, &deployment.Logs, &deployment.ErrorMessage,
			&deployment.StartedAt, &deployment.EndedAt)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

// CreateDeploymentRecord creates a new deployment record
func (s *DeploymentService) CreateDeploymentRecord(db *sql.DB, projectID int, status, commitSHA string) (int, error) {
	result, err := db.Exec(`
		INSERT INTO deployments (project_id, status, commit_sha, started_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, projectID, status, commitSHA)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	return int(id), err
}

// UpdateDeploymentRecord updates a deployment record with final status and logs
func (s *DeploymentService) UpdateDeploymentRecord(db *sql.DB, deploymentID int, status, logs, errorMessage string) error {
	_, err := db.Exec(`
		UPDATE deployments SET status = ?, logs = ?, error_message = ?, ended_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, logs, errorMessage, deploymentID)
	return err
}
