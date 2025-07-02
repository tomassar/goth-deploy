//go:build !linux

package services

import (
	"fmt"
	"os/exec"
	"syscall"
)

// configureLinuxIsolation provides basic isolation for non-Linux platforms
func configureLinuxIsolation(cmd *exec.Cmd, uid, gid int, chrootPath string, enableChroot bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// User/Group isolation (if supported)
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},

		// Process group isolation
		Setpgid: true,
		Pgid:    0,

		// Additional security
		Noctty: true, // No controlling terminal
	}

	// Note: Chroot and namespaces not available on non-Linux platforms
	if enableChroot {
		fmt.Println("Warning: chroot not supported on this platform")
	}
}

// createDeviceNodeLinux is not available on non-Linux platforms
func createDeviceNodeLinux(path string, mode uint32, major, minor int) error {
	return fmt.Errorf("device node creation not supported on this platform")
}
