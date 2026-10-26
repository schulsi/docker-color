//go:build darwin || linux

package wrapper

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/devemio/docker-color-output/internal/config"
	"github.com/devemio/docker-color-output/internal/stdin"
	"github.com/devemio/docker-color-output/pkg/color"
)

func runLiveStats(command *exec.Cmd, output io.Writer) int {
	return runLivePTY(command, output, func(reader io.Reader) error {
		cfg, cfgErr := config.Default()
		if cfgErr == nil {
			color.SetPalette(color.Palette(cfg.Colors))
		}
		return stdin.Stream(ptyReader{Reader: reader}, output, func(rows []string) error {
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
	})
}

func runLiveCommand(command *exec.Cmd, output io.Writer, kind string) int {
	return runLivePTY(command, output, func(reader io.Reader) error {
		return streamLiveLines(reader, output, kind)
	})
}

func runLivePTY(command *exec.Cmd, output io.Writer, consume func(io.Reader) error) int {
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

	streamErr := consume(ptyReader{Reader: master})
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

func streamLiveLines(input io.Reader, output io.Writer, kind string) error {
	cfg, err := config.Default()
	if err != nil {
		return err
	}
	color.SetPalette(color.Palette(cfg.Colors))
	reader := bufio.NewReader(input)
	line := make([]byte, 0, 4096)
	for {
		value, readErr := reader.ReadByte()
		if value == '\r' || value == '\n' {
			if _, err = io.WriteString(output, colorizeLiveLine(string(line), kind)); err != nil {
				return err
			}
			if _, err = output.Write([]byte{value}); err != nil {
				return err
			}
			line = line[:0]
		} else if readErr == nil {
			line = append(line, value)
			if len(line) >= 64*1024 {
				if _, err = output.Write(line); err != nil {
					return err
				}
				line = line[:0]
			}
		}
		if readErr != nil {
			if len(line) > 0 {
				_, err = io.WriteString(output, colorizeLiveLine(string(line), kind))
			}
			if errors.Is(readErr, syscall.EIO) || errors.Is(readErr, io.EOF) {
				return err
			}
			return readErr
		}
	}
}

func colorizeLiveLine(line, kind string) string {
	if line == "" || strings.Contains(line, "\x1b[") {
		return line
	}
	lower := strings.ToLower(line)
	for _, marker := range []string{"error", "failed", "fatal", "denied"} {
		if strings.Contains(lower, marker) {
			return color.Red(line)
		}
	}
	for _, marker := range []string{"created", "started", "running", "removed", "stopped", "done", "complete", "successfully", "built", "pulled"} {
		if strings.Contains(lower, marker) {
			return color.Green(line)
		}
	}
	if kind == "build" && (strings.HasPrefix(line, "#") || strings.HasPrefix(line, "=>") || strings.HasPrefix(line, " =>") || strings.HasPrefix(line, "Step ") || strings.HasPrefix(line, " --->")) {
		return color.Cyan(line)
	}
	if (kind == "compose up" || kind == "compose down") && strings.HasPrefix(line, "[+") {
		return color.LightBlue(line)
	}
	return line
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
