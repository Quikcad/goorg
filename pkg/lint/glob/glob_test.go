package glob

import "testing"

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
		{"cmd/*/*", "cmd/lint/goorg", true},
		{"cmd/*/*", "cmd/lint", false},
		{"*.go", "main.go", true},
		{"*.go", "cmd/lint/goorg/main.go", false},
		{"bin/**", "bin/goorg", true},
	}
	for _, tt := range tests {
		if got := MatchPath(tt.pattern, tt.path); got != tt.want {
			t.Errorf("MatchPath(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

// TestMatchRuleID pins the difference from MatchPath: `*` crosses the slash in
// a rule ID, so a key of `*` means every rule. path.Match would match none,
// which silently disabled every global override before this was split out.
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
		{"dir/max-*", "dir/max-entries", true},
		{"dir/max-*", "dir/domain-layout", false},
	}
	for _, tt := range tests {
		if got := MatchRuleID(tt.pattern, tt.id); got != tt.want {
			t.Errorf("MatchRuleID(%q, %q) = %v, want %v", tt.pattern, tt.id, got, tt.want)
		}
	}
}

func TestSpecificity(t *testing.T) {
	if Specificity("docs/**") <= Specificity("**") {
		t.Error("a literal prefix must outrank a bare wildcard")
	}
	if Specificity("cmd/lint/*") <= Specificity("cmd/*") {
		t.Error("a longer literal prefix must win")
	}
	if Specificity("cmd/lint/goorg") != len("cmd/lint/goorg") {
		t.Error("a pattern with no wildcard scores its full length")
	}
}
