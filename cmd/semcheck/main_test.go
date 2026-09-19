package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// childEnv makes the test binary behave as the semcheck command.
const childEnv = "SEMCHECK_TEST_RUN_MAIN"

// The tests re-execute the test binary instead of building the command: a
// "go build" at run time is invisible to the test cache, which would report a
// cached "ok" after a change in the analyzer.
func TestMain(m *testing.M) {
	if os.Getenv(childEnv) == "1" {
		main() // never returns: singlechecker.Main ends in os.Exit
	}

	os.Exit(m.Run())
}

func command(t *testing.T) string {
	t.Helper()

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	return exe
}

func childEnviron() []string {
	return append(os.Environ(), childEnv+"=1")
}

type result struct {
	stdout, stderr string
	exitCode       int
}

func run(t *testing.T, name string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Env = childEnviron()
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
	"hello.go:5:6: exported-func-doc: matched",
	"hello.go:10:16: exported-func-doc: matched",
	"hello.go:12:16: func-prefix: matched",
	"hello.go:15:16: exported-func-doc: matched",
	"hello.go:15:16: func-prefix: matched",
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
	got := run(t, command(t), "./testdata/hello")

	if got.exitCode != 3 {
		t.Errorf("exit code = %d, want 3", got.exitCode)
	}

	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty: diagnostics go to stderr", got.stdout)
	}

	checkDiagnostics(t, got.stderr)
}

func TestStandaloneClean(t *testing.T) {
	got := run(t, command(t), "./testdata/clean")

	if got != (result{}) {
		t.Errorf("got %+v, want no output and exit code 0", got)
	}
}

func TestStandaloneLoadError(t *testing.T) {
	got := run(t, command(t), "./testdata/does-not-exist")

	if got.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", got.exitCode)
	}

	if got.stderr == "" {
		t.Error("stderr is empty, want an explanation")
	}
}

func TestStandaloneJSON(t *testing.T) {
	got := run(t, command(t), "-json", "./testdata/hello")

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
	got := run(t, "go", "vet", "-vettool="+command(t), "./testdata/hello")

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
