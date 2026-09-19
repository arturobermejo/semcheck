package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binary is the path of the semcheck executable built by TestMain.
var binary string

// TestMain builds the command once, so every test runs the real binary the
// same way a user or CI would.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "semcheck-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	defer func() { _ = os.RemoveAll(dir) }()

	binary = filepath.Join(dir, "semcheck")

	out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "go build: %v\n%s", err, out)
		return 1
	}

	return m.Run()
}

// result is what a process leaves behind: two streams and an exit code.
type result struct {
	stdout, stderr string
	exitCode       int
}

func run(t *testing.T, name string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("%s %v: %v", name, args, err)
	}

	return result{stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()}
}

// wantDiagnostics are the findings for testdata/hello, without the directory
// part of the path: standalone prints absolute paths and go vet relative ones.
var wantDiagnostics = []string{
	"hello.go:5:6: found function Hello",
	"hello.go:9:16: found function greet",
}

func checkDiagnostics(t *testing.T, output string) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != len(wantDiagnostics) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(wantDiagnostics), output)
	}

	for i, want := range wantDiagnostics {
		if !strings.HasSuffix(lines[i], want) {
			t.Errorf("line %d = %q, want suffix %q", i, lines[i], want)
		}
	}
}

func TestStandaloneFindings(t *testing.T) {
	got := run(t, binary, "./testdata/hello")

	if got.exitCode != 3 {
		t.Errorf("exit code = %d, want 3", got.exitCode)
	}

	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty: diagnostics go to stderr", got.stdout)
	}

	checkDiagnostics(t, got.stderr)
}

func TestStandaloneClean(t *testing.T) {
	got := run(t, binary, "./testdata/clean")

	if got != (result{}) {
		t.Errorf("got %+v, want no output and exit code 0", got)
	}
}

func TestStandaloneLoadError(t *testing.T) {
	got := run(t, binary, "./testdata/does-not-exist")

	if got.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", got.exitCode)
	}

	if got.stderr == "" {
		t.Error("stderr is empty, want an explanation")
	}
}

func TestStandaloneJSON(t *testing.T) {
	got := run(t, binary, "-json", "./testdata/hello")

	if got.exitCode != 0 {
		t.Errorf("exit code = %d, want 0: -json never fails on findings", got.exitCode)
	}

	// package path -> analyzer name -> diagnostics
	var report map[string]map[string][]struct {
		Posn    string `json:"posn"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, got.stdout)
	}

	const pkg = "github.com/arturobermejo/semcheck/cmd/semcheck/testdata/hello"

	diags := report[pkg]["semcheck"]
	if len(diags) != len(wantDiagnostics) {
		t.Fatalf("got %d diagnostics, want %d:\n%s", len(diags), len(wantDiagnostics), got.stdout)
	}

	for i, want := range wantDiagnostics {
		if line := diags[i].Posn + ": " + diags[i].Message; !strings.HasSuffix(line, want) {
			t.Errorf("diagnostic %d = %q, want suffix %q", i, line, want)
		}
	}
}

func TestVetTool(t *testing.T) {
	got := run(t, "go", "vet", "-vettool="+binary, "./testdata/hello")

	if got.exitCode != 1 {
		t.Errorf("exit code = %d, want 1: go vet hides the tool's own code", got.exitCode)
	}

	// Depending on the Go version, go vet prints a "# import/path" header
	// line before the findings of each package.
	var findings []string

	for line := range strings.Lines(got.stderr) {
		if !strings.HasPrefix(line, "#") {
			findings = append(findings, line)
		}
	}

	checkDiagnostics(t, strings.Join(findings, ""))
}
