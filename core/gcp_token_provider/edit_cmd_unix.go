//go:build !windows

package gcp_token_provider

import (
	"barbe/core/chown_util"
	"os/exec"
	"syscall"
)

func editCmd(cmd *exec.Cmd) {
	if uid, gid, err := chown_util.GetSudoerUser(); err == nil && uid != -1 && gid != -1 {
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	}
}
