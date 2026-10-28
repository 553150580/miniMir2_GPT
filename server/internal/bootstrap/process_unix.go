//go:build !windows

package bootstrap

import (
	"os/exec"
	"syscall"
)

// 独立进程组，停止只发送给本次启动并持有句柄的子进程。
func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopChild(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}
