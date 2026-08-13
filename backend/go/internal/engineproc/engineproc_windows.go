//go:build windows

package engineproc

import (
	"fmt"
	"os/exec"
	"syscall"
)

func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func killProcGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		exec.Command("taskkill", "/pid", fmt.Sprint(cmd.Process.Pid), "/T", "/F").Run()
	}
}
