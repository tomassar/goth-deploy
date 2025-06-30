package services

import (
	"bufio"
	"context"
	"fmt"
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

	// Start new service
	cmd := exec.CommandContext(ctx, "./app")
	cmd.Dir = projectPath
	cmd.Env = append(os.Environ(),
		"PORT=0", // Let the system assign a port
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

	return cmd.Start()
}

func (s *DeploymentService) StopService(subdomain string) error {
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
