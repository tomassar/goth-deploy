package services

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"goth-deploy/internal/config"
	"goth-deploy/internal/models"
)

// DeploymentService handles project deployments
type DeploymentService struct {
	DB        *sql.DB
	Config    *config.Config
	processes map[string]*exec.Cmd
	mutex     sync.RWMutex
}

// NewDeploymentService creates a new deployment service
func NewDeploymentService(db *sql.DB, cfg *config.Config) *DeploymentService {
	return &DeploymentService{
		DB:        db,
		Config:    cfg,
		processes: make(map[string]*exec.Cmd),
	}
}

// DeployProject deploys a project from GitHub
func (d *DeploymentService) DeployProject(projectID int64, commitSHA string) (*models.Deployment, error) {
	log.Printf("🚀 [DEPLOY] Starting deployment for project ID %d", projectID)
	if commitSHA != "" {
		log.Printf("📋 [DEPLOY] Target commit: %s", commitSHA)
	} else {
		log.Printf("📋 [DEPLOY] Target: latest commit from default branch")
	}

	// Get project details
	var project models.Project
	err := d.DB.QueryRow(`
		SELECT id, user_id, name, repo_url, branch, subdomain, build_command, start_command, port
		FROM projects WHERE id = ?
	`, projectID).Scan(
		&project.ID,
		&project.UserID,
		&project.Name,
		&project.RepoURL,
		&project.Branch,
		&project.Subdomain,
		&project.BuildCommand,
		&project.StartCommand,
		&project.Port,
	)
	if err != nil {
		log.Printf("❌ [DEPLOY] Failed to get project details for ID %d: %v", projectID, err)
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	log.Printf("📦 [DEPLOY] Project: %s (subdomain: %s)", project.Name, project.Subdomain)
	log.Printf("🔗 [DEPLOY] Repository: %s (branch: %s)", project.RepoURL, project.Branch)
	log.Printf("🔨 [DEPLOY] Build command: %s", project.BuildCommand)
	log.Printf("▶️  [DEPLOY] Start command: %s", project.StartCommand)
	log.Printf("🌐 [DEPLOY] Port: %d", project.Port)

	// Create deployment record
	log.Printf("💾 [DEPLOY] Creating deployment record...")
	result, err := d.DB.Exec(`
		INSERT INTO deployments (project_id, commit_sha, status, started_at, created_at)
		VALUES (?, ?, 'pending', ?, ?)
	`, projectID, commitSHA, time.Now(), time.Now())
	if err != nil {
		log.Printf("❌ [DEPLOY] Failed to create deployment record: %v", err)
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	deploymentID, err := result.LastInsertId()
	if err != nil {
		log.Printf("❌ [DEPLOY] Failed to get deployment ID: %v", err)
		return nil, fmt.Errorf("failed to get deployment ID: %w", err)
	}

	deployment := &models.Deployment{
		ID:        deploymentID,
		ProjectID: projectID,
		CommitSHA: commitSHA,
		Status:    models.StatusPending,
		StartedAt: time.Now(),
		CreatedAt: time.Now(),
	}

	log.Printf("✅ [DEPLOY] Deployment record created with ID %d", deploymentID)

	// Update project status
	log.Printf("🔄 [DEPLOY] Updating project status to 'building'...")
	_, err = d.DB.Exec("UPDATE projects SET status = 'building' WHERE id = ?", projectID)
	if err != nil {
		log.Printf("⚠️  [DEPLOY] Failed to update project status: %v", err)
		return deployment, fmt.Errorf("failed to update project status: %w", err)
	}

	// Start deployment in background
	log.Printf("🎯 [DEPLOY] Starting deployment process in background...")
	go d.performDeployment(deployment, &project)

	return deployment, nil
}

// performDeployment performs the actual deployment process
func (d *DeploymentService) performDeployment(deployment *models.Deployment, project *models.Project) {
	startTime := time.Now()
	log.Printf("🎬 [DEPLOY-%d] Starting deployment process for project '%s'", deployment.ID, project.Name)

	var buildLog strings.Builder
	var err error
	var startDuration time.Duration

	// Add deployment header to build log
	buildLog.WriteString(fmt.Sprintf("=== Deployment #%d for %s ===\n", deployment.ID, project.Name))
	buildLog.WriteString(fmt.Sprintf("Started: %s\n", startTime.Format(time.RFC3339)))
	buildLog.WriteString(fmt.Sprintf("Repository: %s\n", project.RepoURL))
	buildLog.WriteString(fmt.Sprintf("Branch: %s\n", project.Branch))
	buildLog.WriteString(fmt.Sprintf("Subdomain: %s\n", project.Subdomain))
	buildLog.WriteString("===========================================\n\n")

	// Update deployment status to building
	log.Printf("📝 [DEPLOY-%d] Updating deployment status to 'building'", deployment.ID)
	d.updateDeploymentStatus(deployment.ID, models.StatusBuilding, "", "")

	defer func() {
		duration := time.Since(startTime)
		if err != nil {
			log.Printf("💥 [DEPLOY-%d] Deployment FAILED after %v: %v", deployment.ID, duration, err)
			buildLog.WriteString(fmt.Sprintf("\n=== DEPLOYMENT FAILED ===\n"))
			buildLog.WriteString(fmt.Sprintf("Duration: %v\n", duration))
			buildLog.WriteString(fmt.Sprintf("Error: %v\n", err))
			d.updateDeploymentStatus(deployment.ID, models.StatusFailed, buildLog.String(), err.Error())
			d.updateProjectStatus(project.ID, models.ProjectStatusFailed)
		} else {
			log.Printf("🎉 [DEPLOY-%d] Deployment SUCCESSFUL in %v", deployment.ID, duration)
			buildLog.WriteString(fmt.Sprintf("\n=== DEPLOYMENT SUCCESSFUL ===\n"))
			buildLog.WriteString(fmt.Sprintf("Duration: %v\n", duration))
			buildLog.WriteString(fmt.Sprintf("Application available at: http://%s.%s\n", project.Subdomain, d.Config.BaseDomain))
			d.updateDeploymentStatus(deployment.ID, models.StatusSuccess, buildLog.String(), "")
			d.updateProjectStatus(project.ID, models.ProjectStatusActive)
			// Update last deploy time
			d.DB.Exec("UPDATE projects SET last_deploy = ? WHERE id = ?", time.Now(), project.ID)
		}
	}()

	// Stop any existing process for this project
	log.Printf("🛑 [DEPLOY-%d] Stopping any existing processes for subdomain '%s'", deployment.ID, project.Subdomain)
	buildLog.WriteString("🛑 Stopping existing processes...\n")
	d.stopProjectProcess(project.Subdomain)
	buildLog.WriteString("✅ Existing processes stopped\n\n")

	// Create deployment directory
	deployDir := filepath.Join(d.Config.DeploymentRoot, project.Subdomain)
	log.Printf("📁 [DEPLOY-%d] Setting up deployment directory: %s", deployment.ID, deployDir)
	buildLog.WriteString(fmt.Sprintf("📁 Setting up deployment directory: %s\n", deployDir))

	if err = os.RemoveAll(deployDir); err != nil {
		log.Printf("⚠️  [DEPLOY-%d] Warning: Failed to remove existing directory: %v", deployment.ID, err)
		buildLog.WriteString(fmt.Sprintf("⚠️  Warning: Failed to remove existing directory: %v\n", err))
		// Continue anyway - this might be the first deployment
	}

	if err = os.MkdirAll(deployDir, 0755); err != nil {
		log.Printf("❌ [DEPLOY-%d] Failed to create deployment directory: %v", deployment.ID, err)
		buildLog.WriteString(fmt.Sprintf("❌ Failed to create deployment directory: %v\n", err))
		return
	}
	log.Printf("✅ [DEPLOY-%d] Deployment directory created successfully", deployment.ID)
	buildLog.WriteString("✅ Deployment directory created\n\n")

	// Clone repository
	log.Printf("📥 [DEPLOY-%d] Cloning repository %s (branch: %s)", deployment.ID, project.RepoURL, project.Branch)
	buildLog.WriteString(fmt.Sprintf("📥 Cloning repository %s (branch: %s)...\n", project.RepoURL, project.Branch))

	cloneStart := time.Now()
	cloneCmd := exec.Command("git", "clone", "--branch", project.Branch, "--single-branch", project.RepoURL, deployDir)
	cloneOutput, cloneErr := cloneCmd.CombinedOutput()
	cloneDuration := time.Since(cloneStart)

	buildLog.WriteString(string(cloneOutput))
	if cloneErr != nil {
		log.Printf("❌ [DEPLOY-%d] Git clone failed after %v: %v", deployment.ID, cloneDuration, cloneErr)
		buildLog.WriteString(fmt.Sprintf("❌ Git clone failed: %v\n", cloneErr))
		err = fmt.Errorf("git clone failed: %w", cloneErr)
		return
	}
	log.Printf("✅ [DEPLOY-%d] Repository cloned successfully in %v", deployment.ID, cloneDuration)
	buildLog.WriteString(fmt.Sprintf("✅ Repository cloned in %v\n\n", cloneDuration))

	// Verify deployment directory exists
	log.Printf("📂 [DEPLOY-%d] Verifying deployment directory: %s", deployment.ID, deployDir)
	if _, err = os.Stat(deployDir); os.IsNotExist(err) {
		log.Printf("❌ [DEPLOY-%d] Deployment directory does not exist: %s", deployment.ID, deployDir)
		buildLog.WriteString(fmt.Sprintf("❌ Deployment directory does not exist: %s\n", deployDir))
		err = fmt.Errorf("deployment directory does not exist: %s", deployDir)
		return
	}
	log.Printf("✅ [DEPLOY-%d] Deployment directory verified", deployment.ID)
	buildLog.WriteString("✅ Deployment directory verified\n\n")

	// Checkout specific commit if provided
	if deployment.CommitSHA != "" {
		log.Printf("🔄 [DEPLOY-%d] Checking out specific commit: %s", deployment.ID, deployment.CommitSHA)
		buildLog.WriteString(fmt.Sprintf("🔄 Checking out commit %s...\n", deployment.CommitSHA))

		checkoutStart := time.Now()
		checkoutCmd := exec.Command("git", "checkout", deployment.CommitSHA)
		checkoutCmd.Dir = deployDir
		checkoutOutput, checkoutErr := checkoutCmd.CombinedOutput()
		checkoutDuration := time.Since(checkoutStart)

		buildLog.WriteString(string(checkoutOutput))
		if checkoutErr != nil {
			log.Printf("❌ [DEPLOY-%d] Git checkout failed after %v: %v", deployment.ID, checkoutDuration, checkoutErr)
			buildLog.WriteString(fmt.Sprintf("❌ Git checkout failed: %v\n", checkoutErr))
			err = fmt.Errorf("git checkout failed: %w", checkoutErr)
			return
		}
		log.Printf("✅ [DEPLOY-%d] Checked out commit successfully in %v", deployment.ID, checkoutDuration)
		buildLog.WriteString(fmt.Sprintf("✅ Checked out commit in %v\n\n", checkoutDuration))
	} else {
		log.Printf("ℹ️  [DEPLOY-%d] Using latest commit from branch %s", deployment.ID, project.Branch)
		buildLog.WriteString(fmt.Sprintf("ℹ️  Using latest commit from branch %s\n\n", project.Branch))
	}

	// Load environment variables
	log.Printf("🔧 [DEPLOY-%d] Loading environment variables for project", deployment.ID)
	envVars := d.getProjectEnvironmentVariables(project.ID)
	log.Printf("ℹ️  [DEPLOY-%d] Loaded %d environment variables", deployment.ID, len(envVars))
	buildLog.WriteString(fmt.Sprintf("🔧 Loaded %d environment variables\n\n", len(envVars)))

	// Build the project
	log.Printf("🔨 [DEPLOY-%d] Starting build with command: %s", deployment.ID, project.BuildCommand)
	buildLog.WriteString(fmt.Sprintf("🔨 Building project with command: %s...\n", project.BuildCommand))

	buildParts := strings.Fields(project.BuildCommand)
	if len(buildParts) == 0 {
		log.Printf("❌ [DEPLOY-%d] Build command is empty", deployment.ID)
		buildLog.WriteString("❌ Build command is empty\n")
		err = fmt.Errorf("build command is empty")
		return
	}

	buildStart := time.Now()
	buildCmd := exec.Command(buildParts[0], buildParts[1:]...)
	buildCmd.Env = append(os.Environ(), envVars...)
	buildCmd.Dir = deployDir

	log.Printf("📋 [DEPLOY-%d] Build environment: %d total env vars", deployment.ID, len(buildCmd.Env))
	buildOutput, buildErr := buildCmd.CombinedOutput()
	buildDuration := time.Since(buildStart)

	buildLog.WriteString(string(buildOutput))
	if buildErr != nil {
		log.Printf("❌ [DEPLOY-%d] Build failed after %v: %v", deployment.ID, buildDuration, buildErr)
		buildLog.WriteString(fmt.Sprintf("❌ Build failed after %v: %v\n", buildDuration, buildErr))
		err = fmt.Errorf("build failed: %w", buildErr)
		return
	}

	log.Printf("✅ [DEPLOY-%d] Build completed successfully in %v", deployment.ID, buildDuration)
	buildLog.WriteString(fmt.Sprintf("✅ Build completed successfully in %v!\n\n", buildDuration))

	// Start the application
	log.Printf("🚀 [DEPLOY-%d] Starting application with command: %s", deployment.ID, project.StartCommand)
	buildLog.WriteString(fmt.Sprintf("🚀 Starting application with command: %s...\n", project.StartCommand))

	appStartTime := time.Now()
	if startErr := d.startApplication(project, deployDir, envVars); startErr != nil {
		log.Printf("❌ [DEPLOY-%d] Failed to start application: %v", deployment.ID, startErr)
		err = fmt.Errorf("failed to start application: %w", startErr)
		buildLog.WriteString(fmt.Sprintf("❌ Failed to start application: %v\n", startErr))
		return
	}
	startDuration = time.Since(appStartTime)

	log.Printf("🎉 [DEPLOY-%d] Application started successfully on port %d in %v", deployment.ID, project.Port, startDuration)
	log.Printf("🌐 [DEPLOY-%d] Project available at: http://%s.%s", deployment.ID, project.Subdomain, d.Config.BaseDomain)
	buildLog.WriteString(fmt.Sprintf("🎉 Application started successfully on port %d in %v!\n", project.Port, startDuration))
	buildLog.WriteString(fmt.Sprintf("🌐 Project is now available at: http://%s.%s\n", project.Subdomain, d.Config.BaseDomain))
}

// startApplication starts the application process
func (d *DeploymentService) startApplication(project *models.Project, deployDir string, envVars []string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Parse start command
	startParts := strings.Fields(project.StartCommand)
	if len(startParts) == 0 {
		return fmt.Errorf("empty start command")
	}

	// Create command
	cmd := exec.Command(startParts[0], startParts[1:]...)
	cmd.Dir = deployDir

	// Set environment variables
	cmd.Env = append(os.Environ(), envVars...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PORT=%d", project.Port))

	// Create log files for stdout/stderr
	logDir := filepath.Join(deployDir, "logs")
	os.MkdirAll(logDir, 0755)

	stdoutFile, err := os.Create(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		return fmt.Errorf("failed to create stdout log: %w", err)
	}

	stderrFile, err := os.Create(filepath.Join(logDir, "stderr.log"))
	if err != nil {
		stdoutFile.Close()
		return fmt.Errorf("failed to create stderr log: %w", err)
	}

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	// Start the process
	if err := cmd.Start(); err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Store the process
	d.processes[project.Subdomain] = cmd

	// Monitor the process in a goroutine
	go func() {
		defer stdoutFile.Close()
		defer stderrFile.Close()

		err := cmd.Wait()

		d.mutex.Lock()
		delete(d.processes, project.Subdomain)
		d.mutex.Unlock()

		if err != nil {
			// Application crashed, update project status
			d.updateProjectStatus(project.ID, models.ProjectStatusFailed)
			fmt.Printf("Application %s crashed: %v\n", project.Subdomain, err)
		} else {
			// Application stopped gracefully
			d.updateProjectStatus(project.ID, models.ProjectStatusInactive)
			fmt.Printf("Application %s stopped\n", project.Subdomain)
		}
	}()

	// Wait a moment for the app to start
	time.Sleep(2 * time.Second)

	// Check if the process is still running
	if cmd.Process == nil || cmd.ProcessState != nil {
		return fmt.Errorf("application failed to start")
	}

	return nil
}

// stopProjectProcess stops a running project process
func (d *DeploymentService) stopProjectProcess(subdomain string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if cmd, exists := d.processes[subdomain]; exists {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		delete(d.processes, subdomain)
	}
}

// IsProjectRunning checks if a project's application is currently running
func (d *DeploymentService) IsProjectRunning(subdomain string) bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	cmd, exists := d.processes[subdomain]
	if !exists {
		return false
	}

	// Check if process is still alive
	return cmd.Process != nil && cmd.ProcessState == nil
}

// RestartProject restarts a project's application
func (d *DeploymentService) RestartProject(projectID int64) error {
	// Get project details
	var project models.Project
	err := d.DB.QueryRow(`
		SELECT id, user_id, name, repo_url, branch, subdomain, build_command, start_command, port
		FROM projects WHERE id = ?
	`, projectID).Scan(
		&project.ID,
		&project.UserID,
		&project.Name,
		&project.RepoURL,
		&project.Branch,
		&project.Subdomain,
		&project.BuildCommand,
		&project.StartCommand,
		&project.Port,
	)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Stop current process
	d.stopProjectProcess(project.Subdomain)

	// Start the application
	deployDir := filepath.Join(d.Config.DeploymentRoot, project.Subdomain)
	envVars := d.getProjectEnvironmentVariables(project.ID)

	if err := d.startApplication(&project, deployDir, envVars); err != nil {
		d.updateProjectStatus(project.ID, models.ProjectStatusFailed)
		return err
	}

	d.updateProjectStatus(project.ID, models.ProjectStatusActive)
	return nil
}

// updateDeploymentStatus updates the deployment status in the database
func (d *DeploymentService) updateDeploymentStatus(deploymentID int64, status, buildLog, errorMsg string) {
	finishedAt := time.Now()
	_, err := d.DB.Exec(`
		UPDATE deployments 
		SET status = ?, build_log = ?, error_msg = ?, finished_at = ?
		WHERE id = ?
	`, status, buildLog, errorMsg, finishedAt, deploymentID)
	if err != nil {
		fmt.Printf("Failed to update deployment status: %v\n", err)
	}
}

// updateProjectStatus updates the project status in the database
func (d *DeploymentService) updateProjectStatus(projectID int64, status string) {
	_, err := d.DB.Exec("UPDATE projects SET status = ? WHERE id = ?", status, projectID)
	if err != nil {
		fmt.Printf("Failed to update project status: %v\n", err)
	}
}

// getProjectEnvironmentVariables retrieves environment variables for a project
func (d *DeploymentService) getProjectEnvironmentVariables(projectID int64) []string {
	rows, err := d.DB.Query("SELECT key, value FROM environment_variables WHERE project_id = ?", projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var envVars []string
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}

	return envVars
}

// GetDeploymentLogs retrieves build logs for a deployment
func (d *DeploymentService) GetDeploymentLogs(deploymentID int64) (string, error) {
	var buildLog string
	err := d.DB.QueryRow("SELECT build_log FROM deployments WHERE id = ?", deploymentID).Scan(&buildLog)
	if err != nil {
		return "", fmt.Errorf("failed to get deployment logs: %w", err)
	}
	return buildLog, nil
}

// GetApplicationLogs retrieves runtime logs for a project
func (d *DeploymentService) GetApplicationLogs(projectID int64) (stdout, stderr string, err error) {
	// Get project subdomain
	var subdomain string
	err = d.DB.QueryRow("SELECT subdomain FROM projects WHERE id = ?", projectID).Scan(&subdomain)
	if err != nil {
		return "", "", fmt.Errorf("failed to get project: %w", err)
	}

	deployDir := filepath.Join(d.Config.DeploymentRoot, subdomain)
	logDir := filepath.Join(deployDir, "logs")

	// Read stdout log
	stdoutBytes, err := os.ReadFile(filepath.Join(logDir, "stdout.log"))
	if err != nil {
		stdout = "No stdout log available"
	} else {
		stdout = string(stdoutBytes)
	}

	// Read stderr log
	stderrBytes, err := os.ReadFile(filepath.Join(logDir, "stderr.log"))
	if err != nil {
		stderr = "No stderr log available"
	} else {
		stderr = string(stderrBytes)
	}

	return stdout, stderr, nil
}

// StopProject stops a running project
func (d *DeploymentService) StopProject(projectID int64) error {
	// Get project subdomain
	var subdomain string
	err := d.DB.QueryRow("SELECT subdomain FROM projects WHERE id = ?", projectID).Scan(&subdomain)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	// Stop the process
	d.stopProjectProcess(subdomain)

	// Update project status to inactive
	d.updateProjectStatus(projectID, models.ProjectStatusInactive)

	return nil
}

// DeleteProject removes a project and its deployments
func (d *DeploymentService) DeleteProject(projectID int64) error {
	// Get project details for cleanup
	var subdomain string
	err := d.DB.QueryRow("SELECT subdomain FROM projects WHERE id = ?", projectID).Scan(&subdomain)
	if err != nil {
		return fmt.Errorf("failed to get project details: %w", err)
	}

	// Stop the process
	d.stopProjectProcess(subdomain)

	// Remove deployment directory
	deployDir := filepath.Join(d.Config.DeploymentRoot, subdomain)
	os.RemoveAll(deployDir)

	// Delete from database (cascades to deployments and env vars)
	_, err = d.DB.Exec("DELETE FROM projects WHERE id = ?", projectID)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	return nil
}
