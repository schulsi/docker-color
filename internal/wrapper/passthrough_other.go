//go:build !darwin && !linux

package wrapper

import (
	"fmt"
	"os/exec"
)

func runPassthrough(command *exec.Cmd) int {
	if err := command.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(command.Stderr, "dcolor: execute Docker:", err)
		return 126
	}
	return 0
}
