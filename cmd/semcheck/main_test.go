package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/arturobermejo/semcheck"
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

// The child answers every question with the same probability: what is under
// test is the command, not the rules.
func childEnviron() []string {
	return append(os.Environ(), childEnv+"=1", semcheck.JudgeEnv+"=fake:0.95")
}

type result struct {
	stdout, stderr string
	exitCode       int
}

func run(t *testing.T, name string, args ...string) result {
	t.Helper()

	return runWith(t, childEnviron(), name, args...)
}

func runWith(t *testing.T, env []string, name string, args ...string) result {
	t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := exec.Command(name, args...)
	cmd.Env = env
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
	"hello.go:14:16: name-matches-behavior: the name suggests that the function only reads (0.95)",
	"hello.go:17:16: name-matches-behavior: the name suggests that the function only reads (0.95)",
	"hello.go:20:2: no-pii-in-logs: this log includes personal data (0.95)",
	"hello_test.go:5:6: test-name-matches: the name does not say what the test checks (0.95)",
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
	got := run(t, command(t), "-config=testdata/.semcheck.yml", "./testdata/hello")

	if got.exitCode != 3 {
		t.Errorf("exit code = %d, want 3", got.exitCode)
	}

	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty: diagnostics go to stderr", got.stdout)
	}

	checkDiagnostics(t, got.stderr)
}

func TestStandaloneClean(t *testing.T) {
	got := run(t, command(t), "-config=testdata/.semcheck.yml", "./testdata/clean")

	if got != (result{}) {
		t.Errorf("got %+v, want no output and exit code 0", got)
	}
}

func TestStandaloneLoadError(t *testing.T) {
	got := run(t, command(t), "-config=testdata/.semcheck.yml", "./testdata/does-not-exist")

	if got.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", got.exitCode)
	}

	if got.stderr == "" {
		t.Error("stderr is empty, want an explanation")
	}
}

func TestStandaloneJSON(t *testing.T) {
	got := run(t, command(t), "-json", "-config=testdata/.semcheck.yml", "./testdata/hello")

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

	// A package with tests is analyzed twice. Only the test variant contains
	// hello_test.go; the text output hides the repetition, JSON does not.
	const (
		path = "github.com/arturobermejo/semcheck/cmd/semcheck/testdata/hello"
		pkg  = path + " [" + path + ".test]"
	)

	if len(report) != 2 {
		t.Errorf("got %d package variants, want 2:\n%s", len(report), got.stdout)
	}

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

func TestStandaloneConfigErrors(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{"a file that does not exist", "testdata/missing.yml", "testdata/missing.yml"},
		{"a file with invalid rules", "testdata/invalid.yml", `rule #1 "r": match: unknown matcher "http-handler"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, command(t), "-config="+tt.config, "./testdata/hello")

			if got.exitCode != 1 {
				t.Errorf("exit code = %d, want 1: nothing could be analyzed", got.exitCode)
			}

			if !strings.Contains(got.stderr, tt.want) {
				t.Errorf("stderr does not mention %q:\n%s", tt.want, got.stderr)
			}

			if strings.Contains(got.stderr, "semcheck: semcheck:") {
				t.Errorf("the prefix shows twice:\n%s", got.stderr)
			}
		})
	}
}

func TestStandaloneWithoutModel(t *testing.T) {
	env := append(os.Environ(), childEnv+"=1", semcheck.JudgeEnv+"=")

	got := runWith(t, env, command(t), "-config=testdata/.semcheck.yml", "./testdata/hello")

	if got.exitCode != 1 || !strings.Contains(got.stderr, semcheck.JudgeEnv) {
		t.Errorf("got %+v, want exit code 1 and a hint about %s", got, semcheck.JudgeEnv)
	}

	// A package without anything to ask about does not need a model.
	got = runWith(t, env, command(t), "-config=testdata/.semcheck.yml", "./testdata/clean")
	if got != (result{}) {
		t.Errorf("got %+v, want no output and exit code 0", got)
	}
}

// The fixtures label each log with a letter. The ones that must be reported
// are those golangci-lint v2.13.2 reported for the same files with semcheck as
// a plugin: the command has to honor //nolint the way golangci-lint does.
func TestStandaloneNolint(t *testing.T) {
	got := run(t, command(t), "-c=0", "-config=testdata/.semcheck.yml", "./testdata/nolint", "./testdata/nolintfile")

	if got.exitCode != 3 {
		t.Errorf("exit code = %d, want 3", got.exitCode)
	}

	var labels []string
	for _, m := range regexp.MustCompile(`Println\("([A-Z]) `).FindAllStringSubmatch(got.stderr, -1) {
		labels = append(labels, m[1])
	}

	slices.Sort(labels)

	if reported := strings.Join(labels, ""); reported != "ACEHIOSUW" {
		t.Errorf("reported %s, want ACEHIOSUW:\n%s", reported, got.stderr)
	}
}
