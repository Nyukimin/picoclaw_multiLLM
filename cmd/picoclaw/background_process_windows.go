//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	windowsDetachedProcess    = 0x00000008
	windowsCreateNoWindow     = 0x08000000
	windowsNewProcessGroup    = 0x00000200
	windowsBackgroundProcFlag = windowsDetachedProcess | windowsCreateNoWindow | windowsNewProcessGroup
)

func prepareBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsBackgroundProcFlag}
}
