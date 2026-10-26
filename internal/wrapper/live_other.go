//go:build !darwin && !linux

package wrapper

import (
	"io"
	"os/exec"
)

func runLiveStats(command *exec.Cmd, output io.Writer) int {
	command.Stdout = output
	return runDirect(command)
}

func runLiveCommand(command *exec.Cmd, output io.Writer, _ string) int {
	command.Stdout = output
	return runDirect(command)
}
