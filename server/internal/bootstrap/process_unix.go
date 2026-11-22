//go:build !windows

// 本文件在 Unix 平台向监管组的节点转交终止信号。

package bootstrap

import (
	"os/exec"
	"syscall"
)

// 独立进程组，停止只发送给本次启动并持有句柄的子进程。
// cmd 记录“cmd”。
func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stopChild 停止子进程。
// cmd 记录“cmd”。
func stopChild(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}
