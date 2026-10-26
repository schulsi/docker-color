package wrapper

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{name: "ps", args: []string{"ps", "-a"}, want: "ps", ok: true},
		{name: "global context", args: []string{"--context", "production", "ps"}, want: "ps", ok: true},
		{name: "global host", args: []string{"-H", "unix:///var/run/docker.sock", "images"}, want: "images", ok: true},
		{name: "container ls", args: []string{"container", "ls"}, want: "ps", ok: true},
		{name: "image ls", args: []string{"image", "ls"}, want: "images", ok: true},
		{name: "compose ps", args: []string{"compose", "ps"}, want: "compose ps", ok: true},
		{name: "compose logs", args: []string{"compose", "logs", "-f"}, want: "compose logs"},
		{name: "stats", args: []string{"stats"}, want: "stats", ok: true},
		{name: "exec passthrough", args: []string{"exec", "-it", "container", "sh"}},
		{name: "unknown passthrough", args: []string{"unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := DetectCommand(test.args)
			if got != test.want || ok != test.ok {
				t.Fatalf("DetectCommand(%q) = (%q, %t), want (%q, %t)", test.args, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestColorEnabledPriority(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		terminal bool
		want     bool
	}{
		{name: "terminal auto", terminal: true, want: true},
		{name: "redirected auto", want: false},
		{name: "explicit always wins", env: map[string]string{"DCOLOR_COLOR": "always", "NO_COLOR": "1"}, want: true},
		{name: "explicit never wins", env: map[string]string{"DCOLOR_COLOR": "never", "CLICOLOR_FORCE": "1"}, terminal: true},
		{name: "no color overrides force", env: map[string]string{"NO_COLOR": "1", "CLICOLOR_FORCE": "1"}, want: false},
		{name: "clicolor off overrides force", env: map[string]string{"CLICOLOR": "0", "CLICOLOR_FORCE": "1"}, want: false},
		{name: "force color", env: map[string]string{"CLICOLOR_FORCE": "1"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				value, ok := test.env[key]
				return value, ok
			}
			if got := ColorEnabled(lookup, test.terminal); got != test.want {
				t.Fatalf("ColorEnabled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestResolveDockerPath(t *testing.T) {
	directory := t.TempDir()
	docker := filepath.Join(directory, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveDockerPath("", directory, filepath.Join(directory, "dcolor"))
	if err != nil || got != docker {
		t.Fatalf("ResolveDockerPath() = (%q, %v), want (%q, nil)", got, err, docker)
	}
	if _, err = ResolveDockerPath(docker, directory, docker); err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("expected recursion error, got %v", err)
	}
}

func TestColorizedMockDocker(t *testing.T) {
	directory := t.TempDir()
	argumentsPath := filepath.Join(directory, "args")
	mock := filepath.Join(directory, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DCOLOR_TEST_ARGS\"\nprintf '%s\\n' 'CONTAINER ID   IMAGE         COMMAND                  CREATED              STATUS              PORTS                  NAMES' '24c5774ef118   dldash/home   \"/docker-entrypoint…\"   25 seconds ago       Up 24 seconds       0.0.0.0:3000->80/tcp   dldash.home'\nprintf '%s\\n' 'docker warning' >&2\nexit 7\n"
	if err := os.WriteFile(mock, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DCOLOR_DOCKER_PATH", mock)
	t.Setenv("DCOLOR_TEST_ARGS", argumentsPath)
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("DCOLOR_COLOR", "auto")

	args := []string{"--context", "production", "ps", "--filter", "name=api server;prod"}
	var stdout, stderr bytes.Buffer
	code := Main(args, strings.NewReader("input"), &stdout, &stderr)
	if code != 7 {
		t.Fatalf("Main() exit code = %d, want 7", code)
	}
	if !strings.Contains(stdout.String(), "\x1b[1;34mCONTAINER ID") {
		t.Fatalf("expected colorized Docker table, got %q", stdout.String())
	}
	if stderr.String() != "docker warning\n" {
		t.Fatalf("stderr = %q, want Docker warning only", stderr.String())
	}
	gotArgs, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(args, "\n") + "\n"; string(gotArgs) != want {
		t.Fatalf("Docker arguments = %q, want %q", gotArgs, want)
	}
}

func TestPassthroughProcess(t *testing.T) {
	if os.Getenv("DCOLOR_TEST_HELPER") == "1" {
		var args []string
		if err := json.Unmarshal([]byte(os.Getenv("DCOLOR_TEST_ARGS_JSON")), &args); err != nil {
			os.Exit(125)
		}
		os.Exit(Main(args, os.Stdin, os.Stdout, os.Stderr))
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("process replacement is tested on Unix platforms")
	}

	directory := t.TempDir()
	argumentsPath := filepath.Join(directory, "args")
	mock := filepath.Join(directory, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$DCOLOR_TEST_ARGS\"\ni=1\nwhile [ \"$i\" -le 20000 ]; do\n  printf 'docker stdout %s\\n' \"$i\"\n  printf 'docker stderr %s\\n' \"$i\" >&2\n  i=$((i + 1))\ndone\nexit 23\n"
	if err := os.WriteFile(mock, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"exec", "-it", "container name", "sh", "-c", "echo '$HOME'"}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	command := execTestCommand()
	command.Env = append(os.Environ(),
		"DCOLOR_TEST_HELPER=1",
		"DCOLOR_TEST_ARGS_JSON="+string(encodedArgs),
		"DCOLOR_DOCKER_PATH="+mock,
		"DCOLOR_TEST_ARGS="+argumentsPath,
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 23 {
		t.Fatalf("helper exit = %v, want code 23", err)
	}
	if strings.Count(stdout.String(), "docker stdout ") != 20000 || strings.Count(stderr.String(), "docker stderr ") != 20000 {
		t.Fatalf("unexpected passthrough volume: stdout lines=%d stderr lines=%d", strings.Count(stdout.String(), "docker stdout "), strings.Count(stderr.String(), "docker stderr "))
	}
	gotArgs, readErr := os.ReadFile(argumentsPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if want := strings.Join(args, "\n") + "\n"; string(gotArgs) != want {
		t.Fatalf("Docker arguments = %q, want %q", gotArgs, want)
	}
}

func TestWriteColorizedPreservesANSI(t *testing.T) {
	input := []byte("\x1b]0;docker\x07\x1b[31malready colored\x1b[0m\n")
	var output bytes.Buffer
	if _, err := writeColorized(input, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), input) {
		t.Fatalf("ANSI output changed: got %q, want %q", output.Bytes(), input)
	}
}

func execTestCommand() *exec.Cmd {
	return exec.Command(os.Args[0], "-test.run=^TestPassthroughProcess$")
}
