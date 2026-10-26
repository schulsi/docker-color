//go:build darwin || linux

package wrapper

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func runPassthrough(command *exec.Cmd) int {
	if err := syscall.Exec(command.Path, command.Args, os.Environ()); err != nil {
		fmt.Fprintln(command.Stderr, "dcolor: execute Docker:", err)
		return 126
	}
	return 0
}
