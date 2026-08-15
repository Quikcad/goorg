package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// testSet is a stand-in rule set: config must be testable without depending on
// any real rule family, or it would break every time a rule is added.
func testSet(t *testing.T) *rule.Set {
	t.Helper()
	noop := func(*rule.Context) []diag.Diagnostic { return nil }
	set, err := rule.NewSet([]*rule.Rule{
		{ID: "dir/domain-layout", Category: rule.Directory, Default: diag.Error, Check: noop},
		{ID: "dir/max-entries", Category: rule.Directory, Default: diag.Warning, Check: noop},
		{ID: "org/member-order", Category: rule.Organization, Default: diag.Error, Check: noop},
		{ID: "pat/factory-naming", Category: rule.Pattern, Default: diag.Warning, Check: noop},
	})
	if err != nil {
		t.Fatalf("build test set: %v", err)
	}
	return set
}

func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/testdata/**", "pkg/rules/testdata/bad.go", true},
		{"**/testdata/**", "testdata/bad.go", true},
		{"**/testdata/**", "testdata", true},
		{"**/testdata/**", "pkg/rules/rules.go", false},
		{"**/*.pb.go", "api/v1/service.pb.go", true},
		{"**/*.pb.go", "api/v1/service.go", false},
		{"pkg/*", "pkg/lint", true},
		{"pkg/*", "pkg/lint/diag", false},
		{"pkg/**", "pkg/lint/diag", true},
		{"*.go", "main.go", true},
		{"*.go", "cmd/lint/goorg/main.go", false},
	}
	for _, tt := range tests {
		if got := MatchPath(tt.pattern, tt.path); got != tt.want {
			t.Errorf("MatchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// TestMatchRuleID pins the difference from MatchPath: `*` crosses the slash in
// a rule ID, so a key of `*` means every rule. path.Match would match none.
func TestMatchRuleID(t *testing.T) {
	tests := []struct {
		pattern string
		id      string
		want    bool
	}{
		{"*", "dir/domain-layout", true},
		{"dir/*", "dir/domain-layout", true},
		{"dir/*", "org/member-order", false},
		{"*-layout", "dir/domain-layout", true},
		{"dir/domain-layout", "dir/domain-layout", true},
		{"dir/domain-?ayout", "dir/domain-layout", true},
		{"org/*", "dir/domain-layout", false},
	}
	for _, tt := range tests {
		if got := MatchRuleID(tt.pattern, tt.id); got != tt.want {
			t.Errorf("MatchRuleID(%q, %q) = %v, want %v", tt.pattern, tt.id, got, tt.want)
		}
	}
}

func TestExcluderAppliesBuiltins(t *testing.T) {
	exclude := Default().Excluder()
	if !exclude("pkg/rules/testdata/x.go") {
		t.Error("builtin testdata exclusion did not apply")
	}
	if exclude("pkg/rules/rules.go") {
		t.Error("excluded a normal source file")
	}
}

func TestExcluderTreatsBarePatternAsAnyDepth(t *testing.T) {
	cfg := &Config{Version: SchemaVersion, Exclude: []string{"*.pb.go"}}
	if !cfg.Excluder()("api/v1/service.pb.go") {
		t.Error("bare pattern did not match at depth")
	}
}

// TestSeveritySpecificity pins the precedence rule: an exact ID beats a
// category glob, which beats a global glob, regardless of map iteration order.
func TestSeveritySpecificity(t *testing.T) {
	set := testSet(t)
	r := set.Get("dir/domain-layout")

	tests := []struct {
		name string
		in   map[string]string
		want diag.Severity
	}{
		{"no config uses the rule default", nil, diag.Error},
		{"global glob", map[string]string{"*": "warning"}, diag.Warning},
		{"category beats global", map[string]string{"*": "off", "dir/*": "warning"}, diag.Warning},
		{"exact beats category", map[string]string{"dir/*": "off", "dir/domain-layout": "error"}, diag.Error},
		{"exact beats global and category", map[string]string{
			"*": "error", "dir/*": "error", "dir/domain-layout": "off",
		}, diag.Off},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Version: SchemaVersion, Rules: tt.in}
			// Run repeatedly: map iteration order is randomized, so a
			// precedence bug would otherwise be flaky rather than absent.
			for range 20 {
				if got := cfg.Severity(r); got != tt.want {
					t.Fatalf("Severity() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	set := testSet(t)
	tests := []struct {
		name string
		cfg  *Config
		want string
	}{
		{
			name: "missing version",
			cfg:  &Config{Rules: map[string]string{"dir/*": "error"}},
			want: "missing `version:`",
		},
		{
			name: "future version",
			cfg:  &Config{Version: 99},
			want: "unsupported version 99",
		},
		{
			name: "unknown rule",
			cfg:  &Config{Version: SchemaVersion, Rules: map[string]string{"dir/nope": "error"}},
			want: "matches no known rule",
		},
		{
			name: "unknown severity",
			cfg:  &Config{Version: SchemaVersion, Rules: map[string]string{"dir/*": "loud"}},
			want: "unknown severity",
		},
		{
			name: "settings for unknown rule",
			cfg:  &Config{Version: SchemaVersion, Settings: map[string]yaml.Node{"org/nope": {}}},
			want: "no such rule",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate(set)
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestValidateReportsEveryProblem keeps a bad config to one round trip to fix.
func TestValidateReportsEveryProblem(t *testing.T) {
	cfg := &Config{
		Version: SchemaVersion,
		Rules:   map[string]string{"dir/nope": "loud", "org/also-nope": "error"},
	}
	err := cfg.Validate(testSet(t))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"dir/nope", "org/also-nope", "unknown severity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
}

// TestLoadRejectsUnknownFields guards the KnownFields setting: a typo in a
// top-level key must fail loudly rather than silently disabling a rule.
func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".goorg.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nrulez:\n  dir/*: error\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, testSet(t)); err == nil {
		t.Fatal("Load() accepted an unknown top-level field")
	}
}

func TestDecoderForAbsentRule(t *testing.T) {
	if got := Default().DecoderFor("dir/max-entries"); got != nil {
		t.Error("DecoderFor returned a decoder for a rule with no settings")
	}
}

func TestDiscoverReturnsEmptyWhenAbsent(t *testing.T) {
	got, err := Discover(t.TempDir())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got != "" {
		t.Errorf("Discover() = %q, want \"\"", got)
	}
}
