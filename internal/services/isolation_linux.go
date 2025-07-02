//go:build linux

package services

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

// configureLinuxIsolation sets up Linux-specific process isolation
func configureLinuxIsolation(cmd *exec.Cmd, uid, gid int, chrootPath string, enableChroot bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// User/Group isolation
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},

		// Namespace isolation (Linux-specific)
		Cloneflags: unix.CLONE_NEWNS | // Mount namespace
			unix.CLONE_NEWPID | // PID namespace
			unix.CLONE_NEWNET | // Network namespace
			unix.CLONE_NEWIPC | // IPC namespace
			unix.CLONE_NEWUTS, // UTS namespace

		// Process group isolation
		Setpgid: true,
		Pgid:    0,

		// Additional security
		Noctty: true, // No controlling terminal
	}

	// If chroot is enabled, set it up
	if enableChroot {
		cmd.SysProcAttr.Chroot = chrootPath
	}
}

// createDeviceNodeLinux creates a device node using Linux syscalls
func createDeviceNodeLinux(path string, mode uint32, major, minor int) error {
	dev := unix.Mkdev(uint32(major), uint32(minor))
	return unix.Mknod(path, mode, int(dev))
}
