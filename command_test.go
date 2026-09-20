package semcheck

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCommandRecord(t *testing.T) {
	t.Setenv(JudgeEnv, "fake:0.95")
	t.Chdir("testdata/config/project")

	path := filepath.Join(t.TempDir(), "record.jsonl")

	// Two programs, as "go vet" runs one for each package: the second one
	// adds to what the first one wrote.
	for range 2 {
		a := newCommandAnalyzer()
		if err := a.Flags.Set("record", path); err != nil {
			t.Fatal(err)
		}

		if _, err := runOn(t, a, checkedSource); err != nil {
			t.Fatal(err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if records := recordsIn(t, data); len(records) != 4 || !records[3].Finding {
		t.Errorf("records = %+v, want the two findings of each program", records)
	}

	a := newCommandAnalyzer()
	if err := a.Flags.Set("record", filepath.Join(path, "below-a-file")); err != nil {
		t.Fatal(err)
	}

	if _, err := runOn(t, a, checkedSource); err == nil || !strings.HasPrefix(err.Error(), "record: ") {
		t.Errorf("error = %v, want one about the record", err)
	}
}
