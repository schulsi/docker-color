//go:build darwin || linux

package wrapper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/devemio/docker-color-output/internal/config"
	"github.com/devemio/docker-color-output/internal/stdin"
	"github.com/devemio/docker-color-output/pkg/color"
)

func runLiveStats(command *exec.Cmd, output io.Writer) int {
	master, slave, err := openPTY()
	if err != nil {
		command.Stdout = output
		return runDirect(command)
	}
	defer master.Close()
	defer slave.Close()
	_ = resizePTY(master, os.Stdout)
	command.Stdout = slave
	if err = command.Start(); err != nil {
		fmt.Fprintln(command.Stderr, "dcolor:", err)
		return 127
	}
	_ = slave.Close()

	resizeSignals := make(chan os.Signal, 1)
	processSignals := make(chan os.Signal, 1)
	stopResize := make(chan struct{})
	signal.Notify(resizeSignals, syscall.SIGWINCH)
	signal.Notify(processSignals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for {
			select {
			case <-resizeSignals:
				_ = resizePTY(master, os.Stdout)
			case <-stopResize:
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case received := <-processSignals:
				if forwarded, ok := received.(syscall.Signal); ok && command.Process != nil {
					_ = command.Process.Signal(forwarded)
				}
			case <-stopResize:
				return
			}
		}
	}()

	cfg, cfgErr := config.Default()
	if cfgErr == nil {
		color.SetPalette(color.Palette(cfg.Colors))
	}
	streamErr := stdin.Stream(ptyReader{Reader: master}, output, func(rows []string) error {
		if len(rows) == 0 {
			return nil
		}
		formatted := rows
		if cfgErr == nil {
			if colored, formatErr := formatRows(rows, cfg); formatErr == nil {
				formatted = colored
			}
		}
		for _, row := range formatted {
			if _, writeErr := fmt.Fprintln(output, row); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	close(stopResize)
	signal.Stop(resizeSignals)
	signal.Stop(processSignals)
	if streamErr != nil {
		_ = command.Process.Kill()
	}
	waitErr := command.Wait()
	if streamErr != nil && !errors.Is(streamErr, syscall.EIO) {
		fmt.Fprintln(command.Stderr, "dcolor: stream Docker stats:", streamErr)
		return 1
	}
	return exitCode(waitErr)
}

type ptyReader struct {
	io.Reader
}

func (reader ptyReader) Read(buffer []byte) (int, error) {
	count, err := reader.Reader.Read(buffer)
	if errors.Is(err, syscall.EIO) {
		return count, io.EOF
	}
	return count, err
}
