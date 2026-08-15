package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture writes a source tree into a temp dir and returns its path.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// exec runs the CLI with a controlled environment and returns stdout, stderr
// and the exit code.
func exec(t *testing.T, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	e := &Env{
		Stdout: &stdout,
		Stderr: &stderr,
		Getenv: func(k string) string { return env[k] },
	}
	code := Main(e, args)
	return stdout.String(), stderr.String(), code
}

// conforming is a minimal tree laid out the way D2 requires.
var conforming = map[string]string{
	"go.mod":                "module example.com/ok\n\ngo 1.25.0\n",
	"cmd/lint/tool/main.go": "package main\n\nfunc main() {}\n",
	"pkg/billing/invoice/invoice.go": "// Package invoice models an invoice.\n" +
		"package invoice\n\ntype Invoice struct {\n\tID string\n}\n",
}

// TestExitCodes covers the contract CI branches on.
func TestExitCodes(t *testing.T) {
	t.Run("clean tree exits 0", func(t *testing.T) {
		root := fixture(t, conforming)
		stdout, stderr, code := exec(t, nil, "check", "-root", root)
		if code != ExitOK {
			t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitOK, stdout, stderr)
		}
		if !strings.Contains(stdout, "no findings") {
			t.Errorf("stdout = %q, want it to report no findings", stdout)
		}
	})

	t.Run("unparseable file exits 1", func(t *testing.T) {
		root := fixture(t, map[string]string{
			"go.mod":              "module example.com/x\n\ngo 1.25.0\n",
			"pkg/a/broken/br.go":  "package broken\n\nfunc (((\n",
			"pkg/a/fine/fine.go":  "package fine\n",
			"pkg/a/fine/more.go":  "package fine\n",
			"pkg/a/fine/even.go":  "package fine\n",
			"pkg/a/fine/most.go":  "package fine\n",
			"pkg/a/fine/last.go":  "package fine\n",
			"pkg/a/fine/first.go": "package fine\n",
		})
		stdout, _, code := exec(t, nil, "check", "-root", root)
		if code != ExitFindings {
			t.Fatalf("exit = %d, want %d\n%s", code, ExitFindings, stdout)
		}
		if !strings.Contains(stdout, "goorg/parse-error") {
			t.Errorf("a parse error must be reported, not swallowed:\n%s", stdout)
		}
	})

	t.Run("bad config exits 2, not 1", func(t *testing.T) {
		root := fixture(t, map[string]string{
			"go.mod":       "module example.com/x\n\ngo 1.25.0\n",
			".goorg.yaml":  "version: 1\nrules:\n  dir/does-not-exist: error\n",
			"pkg/a/b/b.go": "package b\n",
		})
		_, stderr, code := exec(t, nil, "check", "-root", root)
		if code != ExitError {
			t.Fatalf("exit = %d, want %d (config failure must be distinguishable from findings)", code, ExitError)
		}
		if !strings.Contains(stderr, "matches no known rule") {
			t.Errorf("stderr = %q, want it to name the bad key", stderr)
		}
	})

	t.Run("unknown command exits 2", func(t *testing.T) {
		if _, _, code := exec(t, nil, "frobnicate"); code != ExitError {
			t.Fatalf("exit = %d, want %d", code, ExitError)
		}
	})

	t.Run("unknown format exits 2", func(t *testing.T) {
		root := fixture(t, conforming)
		if _, _, code := exec(t, nil, "check", "-root", root, "-format", "xml"); code != ExitError {
			t.Fatalf("exit = %d, want %d", code, ExitError)
		}
	})
}

// TestSuppressionEndToEnd proves D4 works through the whole pipeline.
func TestSuppressionEndToEnd(t *testing.T) {
	t.Run("reasonless directive is an error", func(t *testing.T) {
		root := fixture(t, map[string]string{
			"go.mod":       "module example.com/s\n\ngo 1.25.0\n",
			"pkg/a/b/b.go": "package b\n\n//goorg:ignore org/member-order\nfunc f() {}\n",
		})
		stdout, _, code := exec(t, nil, "check", "-root", root)
		if code != ExitFindings {
			t.Fatalf("exit = %d, want %d\n%s", code, ExitFindings, stdout)
		}
		if !strings.Contains(stdout, "goorg/invalid-suppression") {
			t.Errorf("stdout does not report the reasonless directive:\n%s", stdout)
		}
	})

	t.Run("unused directive warns but does not fail", func(t *testing.T) {
		root := fixture(t, map[string]string{
			"go.mod":       "module example.com/s\n\ngo 1.25.0\n",
			"pkg/a/b/b.go": "package b\n\n//goorg:ignore org/member-order — stale\nfunc f() {}\n",
		})
		stdout, _, code := exec(t, nil, "check", "-root", root)
		if code != ExitOK {
			t.Fatalf("exit = %d, want %d (a stale suppression is a warning)\n%s", code, ExitOK, stdout)
		}
		if !strings.Contains(stdout, "goorg/stale-suppression") {
			t.Errorf("stdout does not report the unused directive:\n%s", stdout)
		}
	})
}

func TestFormats(t *testing.T) {
	root := fixture(t, map[string]string{
		"go.mod":       "module example.com/f\n\ngo 1.25.0\n",
		"pkg/a/b/b.go": "package b\n\n//goorg:ignore org/member-order\nfunc f() {}\n",
	})

	t.Run("json is valid and self-consistent", func(t *testing.T) {
		stdout, _, _ := exec(t, nil, "check", "-root", root, "-format", "json")
		var doc struct {
			Version     int `json:"version"`
			Diagnostics []struct {
				Rule     string `json:"rule"`
				Severity string `json:"severity"`
				Path     string `json:"path"`
				Message  string `json:"message"`
			} `json:"diagnostics"`
			Summary struct {
				Errors   int `json:"errors"`
				Warnings int `json:"warnings"`
			} `json:"summary"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
		}
		if doc.Version != 1 {
			t.Errorf("version = %d, want 1", doc.Version)
		}
		if len(doc.Diagnostics) != doc.Summary.Errors+doc.Summary.Warnings {
			t.Errorf("summary (%d errors, %d warnings) disagrees with %d diagnostics",
				doc.Summary.Errors, doc.Summary.Warnings, len(doc.Diagnostics))
		}
		for _, d := range doc.Diagnostics {
			if d.Rule == "" || d.Severity == "" || d.Path == "" || d.Message == "" {
				t.Errorf("incomplete diagnostic: %+v", d)
			}
		}
	})

	t.Run("github emits one workflow command per line", func(t *testing.T) {
		stdout, _, _ := exec(t, nil, "check", "-root", root, "-format", "github")
		for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
			if !strings.HasPrefix(line, "::error ") && !strings.HasPrefix(line, "::warning ") {
				t.Errorf("line is not a workflow command: %q", line)
			}
		}
	})

	t.Run("auto detects GitHub Actions", func(t *testing.T) {
		stdout, _, _ := exec(t, map[string]string{"GITHUB_ACTIONS": "true"}, "check", "-root", root)
		if !strings.HasPrefix(stdout, "::") {
			t.Errorf("under GITHUB_ACTIONS, auto format should annotate, got:\n%s", stdout)
		}
	})
}

// TestFlagsAfterPaths guards the argument permutation. Without it,
// `goorg check ./... -format=json` would run with the default format and give
// no sign the flag was ignored.
func TestFlagsAfterPaths(t *testing.T) {
	root := fixture(t, conforming)
	t.Chdir(root)

	t.Run("value flag after path", func(t *testing.T) {
		stdout, _, _ := exec(t, nil, "check", "./...", "-format", "json")
		if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
			t.Errorf("-format after the path was ignored:\n%s", stdout)
		}
	})

	t.Run("inline value flag after path", func(t *testing.T) {
		stdout, _, _ := exec(t, nil, "check", "./...", "-format=json")
		if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
			t.Errorf("-format=json after the path was ignored:\n%s", stdout)
		}
	})

	t.Run("bool flag does not swallow the path", func(t *testing.T) {
		_, _, code := exec(t, nil, "check", "./...", "-brief")
		if code != ExitOK {
			t.Errorf("exit = %d, want %d", code, ExitOK)
		}
	})

	t.Run("everything after -- is a path", func(t *testing.T) {
		if _, _, code := exec(t, nil, "check", "--", "./pkg"); code != ExitOK {
			t.Errorf("exit = %d, want %d", code, ExitOK)
		}
	})
}

func TestInitWritesLoadableConfig(t *testing.T) {
	root := fixture(t, conforming)

	if _, stderr, code := exec(t, nil, "init", "-root", root); code != ExitOK {
		t.Fatalf("init failed: exit %d: %s", code, stderr)
	}
	if _, _, code := exec(t, nil, "init", "-root", root); code != ExitError {
		t.Error("init overwrote an existing config without -force")
	}
	if _, stderr, code := exec(t, nil, "init", "-root", root, "-force"); code != ExitOK {
		t.Fatalf("init -force failed: exit %d: %s", code, stderr)
	}

	// The generated file must satisfy the loader it was generated for.
	if _, stderr, code := exec(t, nil, "check", "-root", root); code == ExitError {
		t.Fatalf("generated config does not load: %s", stderr)
	}
}

func TestRulesAndExplain(t *testing.T) {
	root := fixture(t, conforming)

	stdout, _, code := exec(t, nil, "rules", "-root", root)
	if code != ExitOK {
		t.Fatalf("rules: exit = %d", code)
	}
	for _, want := range []string{"dir/domain-layout", "dir/max-entries", "syntax"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("rules output is missing %q:\n%s", want, stdout)
		}
	}

	t.Run("category filter", func(t *testing.T) {
		stdout, _, code := exec(t, nil, "rules", "-root", root, "-category", "org")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		if strings.Contains(stdout, "dir/domain-layout") {
			t.Errorf("-category=org listed a dir/ rule:\n%s", stdout)
		}
	})

	t.Run("explain a real rule", func(t *testing.T) {
		stdout, _, code := exec(t, nil, "explain", "dir/domain-layout")
		if code != ExitOK {
			t.Fatalf("exit = %d", code)
		}
		for _, want := range []string{"Rationale:", "To fix:", "in effect:", "tier:"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("explain output is missing %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("mistyped rule suggests the real one", func(t *testing.T) {
		_, stderr, code := exec(t, nil, "explain", "dir/domain_layout")
		if code != ExitError {
			t.Errorf("exit = %d, want %d", code, ExitError)
		}
		if !strings.Contains(stderr, "no rule") {
			t.Errorf("stderr = %q", stderr)
		}
	})

	t.Run("unknown category exits 2", func(t *testing.T) {
		if _, _, code := exec(t, nil, "rules", "-category", "nope"); code != ExitError {
			t.Errorf("exit = %d, want %d", code, ExitError)
		}
	})
}

// TestInitListsRegisteredRules proves `goorg init` is generated from the live
// rule set rather than a template that drifts.
func TestInitListsRegisteredRules(t *testing.T) {
	stdout, _, code := exec(t, nil, "init", "-stdout")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"dir/domain-layout:", "dir/max-entries:", "version: 1"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("generated config is missing %q:\n%s", want, stdout)
		}
	}
}

func TestVersion(t *testing.T) {
	stdout, _, code := exec(t, nil, "version", "-short")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Error("version -short printed nothing")
	}

	stdout, _, code = exec(t, nil, "version")
	if code != ExitOK || !strings.Contains(stdout, "rules:") {
		t.Errorf("version: exit = %d, stdout = %q", code, stdout)
	}
}

func TestHelp(t *testing.T) {
	stdout, _, code := exec(t, nil, "--help")
	if code != ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"goorg:ignore", "Exit codes", "check"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output is missing %q", want)
		}
	}
}
