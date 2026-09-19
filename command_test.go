package semcheck

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindConfig(t *testing.T) {
	// root/.semcheck.yml, root/a/b/.semcheck.yml and root/a/b/c/
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")

	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{root, filepath.Join(root, "a", "b")} {
		if err := os.WriteFile(filepath.Join(dir, ConfigFile), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		from string
		want string // directory of the file found
	}{
		{"in the directory itself", root, root},
		{"in the parent", filepath.Join(root, "a"), root},
		{"the closest one wins", deep, filepath.Join(root, "a", "b")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindConfig(tt.from)
			if err != nil {
				t.Fatal(err)
			}

			if want := filepath.Join(tt.want, ConfigFile); got != want {
				t.Errorf("FindConfig(%s) = %s, want %s", tt.from, got, want)
			}
		})
	}

	t.Run("nowhere", func(t *testing.T) {
		empty := t.TempDir()

		got, err := FindConfig(empty)
		if err == nil || got != "" {
			t.Fatalf("FindConfig = %q, %v; want an error", got, err)
		}

		// The search ends at the root of the file system, and says where it began.
		if !strings.Contains(err.Error(), empty) {
			t.Errorf("error %q does not mention %s", err, empty)
		}
	})
}

func TestDefaultJudge(t *testing.T) {
	tests := []struct {
		env  string
		want float64 // the answer of the judge to every question
		err  bool
	}{
		{"", 0, true},
		{"fake:0.95", 0.95, false},
		{"fake:0", 0, false},
		{"fake:1", 1, false},
		{"fake:1.5", 0, true},
		{"fake:-0.1", 0, true},
		{"fake:high", 0, true},
		{"fake:", 0, true},
		{"jev", 0, true},
		{"broken:now", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			t.Setenv(JudgeEnv, tt.env)
			t.Setenv(APIKeyEnv, "")

			judge, err := DefaultJudge()
			if tt.err {
				if err == nil || !strings.Contains(err.Error(), JudgeEnv+"="+tt.env) {
					t.Fatalf("error = %v, want one that shows the variable", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			decisions, err := judge.Decide(context.Background(), questions("a", "b"))

			if err != nil || len(decisions) != 2 || decisions[1].Yes != tt.want {
				t.Errorf("Decide = %v, %v; want %v for every question", decisions, err, tt.want)
			}
		})
	}
}

func TestDefaultJudgeIsJev(t *testing.T) {
	t.Setenv(JudgeEnv, "")
	t.Setenv(APIKeyEnv, testKey)

	judge, err := DefaultJudge()
	if err != nil {
		t.Fatal(err)
	}

	if jev, ok := judge.(*JevJudge); !ok || jev.APIKey != testKey {
		t.Errorf("judge = %#v, want a JevJudge with the key", judge)
	}

	// The variable for trying things out wins: no surprise requests.
	t.Setenv(JudgeEnv, "fake:0.5")

	if judge, _ = DefaultJudge(); judge == nil {
		t.Fatal("no judge")
	} else if _, ok := judge.(*FakeJudge); !ok {
		t.Errorf("judge = %#v, want the fake one", judge)
	}
}

func TestDefaultJudgeWithoutKey(t *testing.T) {
	t.Setenv(JudgeEnv, "")
	t.Setenv(APIKeyEnv, "")

	_, err := DefaultJudge()
	if err == nil || !strings.Contains(err.Error(), APIKeyEnv) || !strings.Contains(err.Error(), JudgeEnv) {
		t.Errorf("error = %v, want one that names both variables", err)
	}
}

func TestCommandAnalyzer(t *testing.T) {
	t.Setenv(JudgeEnv, "fake:0.95")

	t.Run("finds the configuration from the working directory", func(t *testing.T) {
		t.Chdir("testdata/config/project/internal/pkg")

		diagnostics, err := runOn(t, newCommandAnalyzer(), checkedSource)
		if err != nil {
			t.Fatal(err)
		}

		if len(diagnostics) != 2 || diagnostics[0].Category != "found-upwards" {
			t.Errorf("diagnostics = %v, want two of the rule found-upwards", diagnostics)
		}
	})

	t.Run("the flag wins", func(t *testing.T) {
		t.Chdir("testdata/config/project")

		a := newCommandAnalyzer()
		if err := a.Flags.Set("config", "../rules.yml"); err != nil {
			t.Fatal(err)
		}

		diagnostics, err := runOn(t, a, checkedSource)
		if err != nil {
			t.Fatal(err)
		}

		// None of the rules of rules.yml selects anything in checkedSource.
		if len(diagnostics) != 0 {
			t.Errorf("diagnostics = %v, want none", diagnostics)
		}
	})

	t.Run("loads once", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ConfigFile)

		data, err := os.ReadFile("testdata/config/project/.semcheck.yml")
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}

		a := newCommandAnalyzer()
		if err := a.Flags.Set("config", path); err != nil {
			t.Fatal(err)
		}

		if _, err := runOn(t, a, checkedSource); err != nil {
			t.Fatal(err)
		}

		// A driver analyzes many packages with one analyzer: the file is read
		// for the first, not for each.
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}

		if diagnostics, err := runOn(t, a, checkedSource); err != nil || len(diagnostics) != 2 {
			t.Errorf("second run = %v, %v", diagnostics, err)
		}
	})

	t.Run("no configuration", func(t *testing.T) {
		t.Chdir(t.TempDir())

		_, err := runOn(t, newCommandAnalyzer(), checkedSource)
		if err == nil || !strings.Contains(err.Error(), "no "+ConfigFile) {
			t.Fatalf("error = %v", err)
		}

		// The driver puts the name of the analyzer in front of the message.
		if strings.HasPrefix(err.Error(), "semcheck: ") {
			t.Errorf("error %q would be shown with the prefix twice", err)
		}
	})

	t.Run("the cause of an error stays reachable", func(t *testing.T) {
		a := newCommandAnalyzer()
		if err := a.Flags.Set("config", "does-not-exist.yml"); err != nil {
			t.Fatal(err)
		}

		if _, err := runOn(t, a, checkedSource); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("error = %v, want one that wraps fs.ErrNotExist", err)
		}
	})
}

func TestDefaultJudgeBroken(t *testing.T) {
	t.Setenv(JudgeEnv, "broken")

	judge, err := DefaultJudge()
	if err != nil {
		t.Fatal(err)
	}

	if decisions, err := judge.Decide(context.Background(), questions("a")); err == nil {
		t.Errorf("Decide = %v, want an error", decisions)
	}
}
