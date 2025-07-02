package services

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

type IsolationService struct {
	baseUID      int
	baseGID      int
	usersDir     string
	chrootBase   string
	enableChroot bool
}

type IsolationConfig struct {
	ProjectID   int
	Subdomain   string
	ProjectPath string
	Port        int
	Secrets     map[string]string
}

func NewIsolationService(baseUID, baseGID int, usersDir, chrootBase string, enableChroot bool) *IsolationService {
	return &IsolationService{
		baseUID:      baseUID,
		baseGID:      baseGID,
		usersDir:     usersDir,
		chrootBase:   chrootBase,
		enableChroot: enableChroot,
	}
}

// CreateIsolatedProcess creates a new process with proper isolation
func (s *IsolationService) CreateIsolatedProcess(config *IsolationConfig) (*exec.Cmd, error) {
	// Create dedicated user for this deployment
	uid, gid, err := s.getOrCreateUser(config.Subdomain)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Prepare isolated environment
	env, err := s.prepareEnvironment(config)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare environment: %w", err)
	}

	// Setup chroot if enabled
	chrootPath := config.ProjectPath
	if s.enableChroot {
		chrootPath, err = s.setupChroot(config)
		if err != nil {
			return nil, fmt.Errorf("failed to setup chroot: %w", err)
		}
	}

	// Create the command with isolation
	cmd := exec.Command("./app")
	cmd.Dir = chrootPath
	cmd.Env = env

	// Configure platform-specific process isolation
	configureLinuxIsolation(cmd, uid, gid, chrootPath, s.enableChroot)

	return cmd, nil
}

// getOrCreateUser creates or gets a dedicated user for the deployment
func (s *IsolationService) getOrCreateUser(subdomain string) (int, int, error) {
	username := fmt.Sprintf("deploy_%s", subdomain)

	// Check if user already exists
	if u, err := user.Lookup(username); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		return uid, gid, nil
	}

	// Find next available UID/GID
	uid := s.baseUID
	gid := s.baseGID

	for {
		if _, err := user.LookupId(strconv.Itoa(uid)); err != nil {
			break // UID is available
		}
		uid++
		if uid > s.baseUID+10000 { // Safety limit
			return 0, 0, fmt.Errorf("no available UIDs")
		}
	}

	// Create user home directory
	homeDir := filepath.Join(s.usersDir, username)
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return 0, 0, fmt.Errorf("failed to create home directory: %w", err)
	}

	// Create the user using useradd command
	cmd := exec.Command("useradd",
		"--uid", strconv.Itoa(uid),
		"--gid", strconv.Itoa(gid),
		"--home-dir", homeDir,
		"--shell", "/bin/false", // No shell access
		"--no-create-home", // We already created it
		"--system",         // System user
		username,
	)

	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("failed to create user %s: %w", username, err)
	}

	// Set proper ownership
	if err := os.Chown(homeDir, uid, gid); err != nil {
		return 0, 0, fmt.Errorf("failed to set ownership: %w", err)
	}

	fmt.Printf("Created isolated user: %s (UID: %d, GID: %d)\n", username, uid, gid)
	return uid, gid, nil
}

// prepareEnvironment creates a clean, minimal environment for the process
func (s *IsolationService) prepareEnvironment(config *IsolationConfig) ([]string, error) {
	env := []string{
		fmt.Sprintf("PORT=%d", config.Port),
		"PATH=/usr/bin:/bin",
		"HOME=/tmp",
		"USER=nobody",
		"SHELL=/bin/false",
		fmt.Sprintf("PROJECT_ID=%d", config.ProjectID),
		fmt.Sprintf("SUBDOMAIN=%s", config.Subdomain),
	}

	// Add project-specific secrets (already decrypted and filtered)
	for key, value := range config.Secrets {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env, nil
}

// setupChroot creates a minimal chroot environment
func (s *IsolationService) setupChroot(config *IsolationConfig) (string, error) {
	chrootPath := filepath.Join(s.chrootBase, config.Subdomain)

	// Create chroot directory structure
	dirs := []string{
		chrootPath,
		filepath.Join(chrootPath, "bin"),
		filepath.Join(chrootPath, "lib"),
		filepath.Join(chrootPath, "lib64"),
		filepath.Join(chrootPath, "usr", "lib"),
		filepath.Join(chrootPath, "tmp"),
		filepath.Join(chrootPath, "proc"),
		filepath.Join(chrootPath, "dev"),
		filepath.Join(chrootPath, "app"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create chroot dir %s: %w", dir, err)
		}
	}

	// Copy the application binary
	srcApp := filepath.Join(config.ProjectPath, "app")
	dstApp := filepath.Join(chrootPath, "app", "app")
	if err := s.copyFile(srcApp, dstApp); err != nil {
		return "", fmt.Errorf("failed to copy app binary: %w", err)
	}

	// Copy essential libraries (minimal set)
	if err := s.copyEssentialLibs(chrootPath); err != nil {
		return "", fmt.Errorf("failed to copy libraries: %w", err)
	}

	// Mount /dev/null and /dev/zero
	devNull := filepath.Join(chrootPath, "dev", "null")
	devZero := filepath.Join(chrootPath, "dev", "zero")

	if err := createDeviceNodeLinux(devNull, syscall.S_IFCHR, 1, 3); err != nil {
		return "", fmt.Errorf("failed to create /dev/null: %w", err)
	}

	if err := createDeviceNodeLinux(devZero, syscall.S_IFCHR, 1, 5); err != nil {
		return "", fmt.Errorf("failed to create /dev/zero: %w", err)
	}

	return filepath.Join(chrootPath, "app"), nil
}

// copyFile copies a file from src to dst
func (s *IsolationService) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Copy file content
	if _, err := srcFile.WriteTo(dstFile); err != nil {
		return err
	}

	// Copy permissions
	return os.Chmod(dst, srcInfo.Mode())
}

// copyEssentialLibs copies minimal required libraries to chroot
func (s *IsolationService) copyEssentialLibs(chrootPath string) error {
	// Get dynamic library dependencies for the app binary
	appPath := filepath.Join(chrootPath, "app", "app")
	cmd := exec.Command("ldd", appPath)
	output, err := cmd.Output()
	if err != nil {
		// If ldd fails, assume static binary
		return nil
	}

	// Parse ldd output and copy required libraries
	// This is a simplified version - in production you'd want more robust parsing
	lines := string(output)
	_ = lines // TODO: Implement library copying based on ldd output

	// For now, just copy some essential libs if they exist
	essentialLibs := []string{
		"/lib64/ld-linux-x86-64.so.2",
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/lib/x86_64-linux-gnu/libpthread.so.0",
	}

	for _, lib := range essentialLibs {
		if _, err := os.Stat(lib); err == nil {
			dst := filepath.Join(chrootPath, lib)
			s.copyFile(lib, dst)
		}
	}

	return nil
}

// CleanupUser removes a user and their associated resources
func (s *IsolationService) CleanupUser(subdomain string) error {
	username := fmt.Sprintf("deploy_%s", subdomain)

	// Remove user
	cmd := exec.Command("userdel", username)
	if err := cmd.Run(); err != nil {
		fmt.Printf("Warning: failed to remove user %s: %v\n", username, err)
	}

	// Remove home directory
	homeDir := filepath.Join(s.usersDir, username)
	if err := os.RemoveAll(homeDir); err != nil {
		fmt.Printf("Warning: failed to remove home directory %s: %v\n", homeDir, err)
	}

	// Remove chroot directory
	if s.enableChroot {
		chrootPath := filepath.Join(s.chrootBase, subdomain)
		if err := os.RemoveAll(chrootPath); err != nil {
			fmt.Printf("Warning: failed to remove chroot %s: %v\n", chrootPath, err)
		}
	}

	fmt.Printf("Cleaned up isolation resources for %s\n", subdomain)
	return nil
}

// SetResourceLimits sets resource limits for the process
func (s *IsolationService) SetResourceLimits(pid int, subdomain string) error {
	// Create cgroup for this deployment
	cgroupPath := filepath.Join("/sys/fs/cgroup", "deployer", subdomain)

	// Create cgroup directory
	dirs := []string{
		cgroupPath,
		filepath.Join(cgroupPath, "memory"),
		filepath.Join(cgroupPath, "cpu"),
		filepath.Join(cgroupPath, "pids"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create cgroup dir %s: %w", dir, err)
		}
	}

	// Set memory limit (512MB)
	memLimit := filepath.Join(cgroupPath, "memory", "memory.limit_in_bytes")
	if err := os.WriteFile(memLimit, []byte("536870912"), 0644); err != nil {
		fmt.Printf("Warning: failed to set memory limit: %v\n", err)
	}

	// Set CPU limit (50% of one core)
	cpuQuota := filepath.Join(cgroupPath, "cpu", "cpu.cfs_quota_us")
	cpuPeriod := filepath.Join(cgroupPath, "cpu", "cpu.cfs_period_us")
	os.WriteFile(cpuPeriod, []byte("100000"), 0644)
	os.WriteFile(cpuQuota, []byte("50000"), 0644)

	// Set process limit (100 processes)
	pidsMax := filepath.Join(cgroupPath, "pids", "pids.max")
	if err := os.WriteFile(pidsMax, []byte("100"), 0644); err != nil {
		fmt.Printf("Warning: failed to set pids limit: %v\n", err)
	}

	// Add process to cgroup
	procsFile := filepath.Join(cgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		fmt.Printf("Warning: failed to add process to cgroup: %v\n", err)
	}

	fmt.Printf("Applied resource limits for %s (PID: %d)\n", subdomain, pid)
	return nil
}

// CleanupCgroup removes the cgroup for a deployment
func (s *IsolationService) CleanupCgroup(subdomain string) error {
	cgroupPath := filepath.Join("/sys/fs/cgroup", "deployer", subdomain)
	if err := os.RemoveAll(cgroupPath); err != nil {
		fmt.Printf("Warning: failed to remove cgroup %s: %v\n", cgroupPath, err)
	}
	return nil
}
