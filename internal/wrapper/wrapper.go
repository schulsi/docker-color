package wrapper

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devemio/docker-color-output/internal/app"
	"github.com/devemio/docker-color-output/internal/config"
	"github.com/devemio/docker-color-output/internal/rules"
	"github.com/devemio/docker-color-output/pkg/color"
)

const maxColorizeBytes = 8 << 20

var errDockerNotFound = errors.New("Docker executable not found")

type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	dockerPath, err := ResolveDockerPath(os.Getenv("DCOLOR_DOCKER_PATH"), os.Getenv("PATH"), executablePath())
	if err != nil {
		fmt.Fprintln(stderr, "dcolor:", err)
		return 127
	}

	kind, colorizable := DetectCommand(args)
	colorize := colorizable && ColorEnabled(os.LookupEnv, stdoutIsTerminal())
	if colorize && kind == "stats" && !hasNoStream(args) {
		command := exec.Command(dockerPath, args...)
		command.Stdin = stdin
		command.Stderr = stderr
		return runLiveStats(command, stdout)
	}

	command := exec.Command(dockerPath, args...)
	command.Stdin = stdin
	command.Stderr = stderr
	if !colorize {
		command.Stdout = stdout
		return runDirect(command)
	}

	return runColorized(command, stdout, kind)
}

func DetectCommand(args []string) (string, bool) {
	index := commandIndex(args)
	if index >= len(args) {
		return "", false
	}

	command := args[index]
	if index+1 < len(args) {
		subcommand := args[index+1]
		switch command {
		case "container":
			if subcommand == "ls" || subcommand == "list" {
				return "ps", true
			}
			if subcommand == "inspect" {
				return "inspect", true
			}
		case "image":
			if subcommand == "ls" || subcommand == "list" {
				return "images", true
			}
		case "compose":
			if subcommand == "ps" {
				return "compose ps", true
			}
			if subcommand == "logs" {
				return "compose logs", false
			}
		}
	}

	switch command {
	case "ps", "images", "inspect":
		return command, true
	case "stats":
		return command, true
	case "logs":
		return command, false
	default:
		return "", false
	}
}

func ColorEnabled(lookup func(string) (string, bool), terminal bool) bool {
	if value, ok := lookup("DCOLOR_COLOR"); ok {
		switch strings.ToLower(value) {
		case string(ColorAlways):
			return true
		case string(ColorNever):
			return false
		}
	}

	if value, ok := lookup("NO_COLOR"); ok && value != "" {
		return false
	}
	if value, ok := lookup("CLICOLOR"); ok && value == "0" {
		return false
	}
	if value, ok := lookup("CLICOLOR_FORCE"); ok && value != "" && value != "0" {
		return true
	}
	return terminal
}

func ResolveDockerPath(override, pathValue, self string) (string, error) {
	if self == "" {
		self = executablePath()
	}
	if override != "" {
		resolved, err := findExecutable(override, pathValue)
		if err != nil {
			return "", fmt.Errorf("DCOLOR_DOCKER_PATH: %w", err)
		}
		if sameFile(resolved, self) {
			return "", errors.New("DCOLOR_DOCKER_PATH resolves to dcolor itself")
		}
		return resolved, nil
	}

	for _, candidate := range []string{
		"/usr/bin/docker",
		"/usr/local/bin/docker",
		"/opt/homebrew/bin/docker",
		"/opt/local/bin/docker",
	} {
		if isExecutable(candidate) && !sameFile(candidate, self) {
			return candidate, nil
		}
	}

	resolved, err := findExecutable("docker", pathValue)
	if err != nil {
		return "", errDockerNotFound
	}
	if sameFile(resolved, self) {
		return "", errors.New("Docker executable resolves to dcolor itself")
	}
	return resolved, nil
}

func commandIndex(args []string) int {
	valueOptions := map[string]bool{
		"--config": true, "--context": true, "--host": true, "-H": true,
		"--log-level": true, "-l": true, "--tlscacert": true,
		"--tlscert": true, "--tlskey": true,
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			return len(args)
		}
		if valueOptions[arg] {
			index++
			continue
		}
		if strings.HasPrefix(arg, "--") {
			if strings.Contains(arg, "=") {
				continue
			}
			if arg == "--tls" || arg == "--tlsverify" || arg == "--debug" || arg == "--version" {
				continue
			}
			return index
		}
		if strings.HasPrefix(arg, "-") {
			if len(arg) > 2 && (strings.HasPrefix(arg, "-H") || strings.HasPrefix(arg, "-l")) {
				continue
			}
			if arg == "-D" || arg == "-v" {
				continue
			}
			return index
		}
		return index
	}
	return len(args)
}

func hasNoStream(args []string) bool {
	for index := commandIndex(args) + 1; index < len(args); index++ {
		if args[index] == "--no-stream" {
			return true
		}
	}
	return false
}

func runColorized(command *exec.Cmd, output io.Writer, kind string) int {
	pipe, err := command.StdoutPipe()
	if err != nil {
		fmt.Fprintln(command.Stderr, "dcolor: stdout pipe:", err)
		return 1
	}
	if err = command.Start(); err != nil {
		fmt.Fprintln(command.Stderr, "dcolor:", err)
		return 127
	}

	data, readErr := io.ReadAll(io.LimitReader(pipe, maxColorizeBytes+1))
	if readErr != nil {
		_, _ = output.Write(data)
		_, _ = io.Copy(output, pipe)
		_ = command.Wait()
		fmt.Fprintln(command.Stderr, "dcolor: read Docker output:", readErr)
		return 1
	}

	if len(data) > maxColorizeBytes {
		_, _ = output.Write(data)
		_, _ = io.Copy(output, pipe)
		return exitCode(command.Wait())
	}

	if err = command.Wait(); err != nil {
		if _, writeErr := writeColorized(data, output, kind); writeErr != nil {
			fmt.Fprintln(command.Stderr, "dcolor: write output:", writeErr)
		}
		return exitCode(err)
	}
	if _, err = writeColorized(data, output, kind); err != nil {
		fmt.Fprintln(command.Stderr, "dcolor: write output:", err)
		return 1
	}
	return 0
}

func writeColorized(data []byte, output io.Writer, kind string) (int, error) {
	if len(data) == 0 || bytes.Contains(data, []byte("\x1b[")) {
		return output.Write(data)
	}
	if kind == "inspect" {
		return writeJSONColorized(data, output)
	}

	rows := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(rows) == 0 || rows[0] == "" {
		return output.Write(data)
	}
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			return output.Write(data)
		}
	}

	cfg, err := config.Default()
	if err != nil {
		return output.Write(data)
	}
	color.SetPalette(color.Palette(cfg.Colors))
	formatted, err := formatRows(rows, cfg)
	if err != nil {
		return output.Write(data)
	}
	return io.WriteString(output, strings.Join(formatted, "\n")+trailingNewline(data))
}

func writeJSONColorized(data []byte, output io.Writer) (int, error) {
	if !json.Valid(data) {
		return output.Write(data)
	}
	cfg, err := config.Default()
	if err != nil {
		return output.Write(data)
	}
	color.SetPalette(color.Palette(cfg.Colors))

	var result strings.Builder
	for index := 0; index < len(data); {
		if data[index] == '"' {
			end := index + 1
			for end < len(data) {
				if data[end] == '\\' {
					end += 2
					continue
				}
				if data[end] == '"' {
					end++
					break
				}
				end++
			}
			colorName := "green"
			lookahead := end
			for lookahead < len(data) && (data[lookahead] == ' ' || data[lookahead] == '\n' || data[lookahead] == '\r' || data[lookahead] == '\t') {
				lookahead++
			}
			if lookahead < len(data) && data[lookahead] == ':' {
				colorName = "lightBlue"
			}
			result.WriteString(rules.Colorize(colorName, string(data[index:end])))
			index = end
			continue
		}

		if data[index] == '-' || data[index] >= '0' && data[index] <= '9' || data[index] == 't' || data[index] == 'f' || data[index] == 'n' {
			end := index + 1
			for end < len(data) && !isJSONDelimiter(data[end]) {
				end++
			}
			colorName := "yellow"
			if data[index] == 't' || data[index] == 'f' || data[index] == 'n' {
				colorName = "purple"
			}
			result.WriteString(rules.Colorize(colorName, string(data[index:end])))
			index = end
			continue
		}

		result.WriteByte(data[index])
		index++
	}
	return io.WriteString(output, result.String())
}

func isJSONDelimiter(value byte) bool {
	switch value {
	case ' ', '\n', '\r', '\t', ',', '}', ']', ':':
		return true
	default:
		return false
	}
}

func formatRows(rows []string, cfg config.Config) (formatted []string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			formatted = nil
			err = fmt.Errorf("format Docker output: %v", recovered)
		}
	}()
	return app.Run(rows, cfg.Rules, cfg.Layout.HeaderColor)
}

func trailingNewline(data []byte) string {
	if len(data) > 0 && data[len(data)-1] == '\n' {
		return "\n"
	}
	return ""
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func executablePath() string {
	path, _ := os.Executable()
	return path
}

func sameFile(first, second string) bool {
	if first == "" || second == "" {
		return false
	}
	firstPath, firstErr := canonicalPath(first)
	secondPath, secondErr := canonicalPath(second)
	return firstErr == nil && secondErr == nil && filepath.Clean(firstPath) == filepath.Clean(secondPath)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	return abs, nil
}

func findExecutable(name, pathValue string) (string, error) {
	if strings.ContainsRune(name, filepath.Separator) || filepath.IsAbs(name) {
		if !isExecutable(name) {
			return "", fmt.Errorf("%q is not an executable file", name)
		}
		return filepath.Abs(name)
	}
	for _, directory := range filepath.SplitList(pathValue) {
		candidate := filepath.Join(directory, name)
		if isExecutable(candidate) {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("%q was not found in PATH", name)
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

func runDirect(command *exec.Cmd) int {
	return runPassthrough(command)
}
