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
