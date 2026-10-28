package bootstrap

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// 每个节点保有独立控制组；CTRL_BREAK 触发 Go 的优雅中断处理，不强杀资产任务。
func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func stopChild(cmd *exec.Cmd) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))
}
