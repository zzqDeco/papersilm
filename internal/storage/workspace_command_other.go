//go:build !(aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris)

package storage

import (
	"os/exec"
	"time"
)

func configureWorkspaceCommandProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = time.Second
}
