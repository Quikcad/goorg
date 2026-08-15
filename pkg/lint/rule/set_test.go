package rule

import (
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/diag"
)

func noop(*Context) []diag.Diagnostic { return nil }

func TestNewSetSortsAndIndexes(t *testing.T) {
	set, err := NewSet(
		[]*Rule{{ID: "org/member-order", Category: Organization, Check: noop}},
		[]*Rule{{ID: "dir/max-entries", Category: Directory, Check: noop}},
	)
	if err != nil {
		t.Fatalf("NewSet() error = %v", err)
	}
	if got := set.IDs(); got[0] != "dir/max-entries" || got[1] != "org/member-order" {
		t.Errorf("IDs() = %v, want sorted", got)
	}
	if set.Get("dir/max-entries") == nil {
		t.Error("Get() missed a registered rule")
	}
	if set.Get("dir/nope") != nil {
		t.Error("Get() returned a rule that was never registered")
	}
	if set.Len() != 2 {
		t.Errorf("Len() = %d, want 2", set.Len())
	}
}

// TestNewSetRejectsBadRules covers the programmer errors that must fail loudly
// at startup rather than in someone's CI.
func TestNewSetRejectsBadRules(t *testing.T) {
	tests := []struct {
		name  string
		rules []*Rule
		want  string
	}{
		{
			name:  "malformed ID",
			rules: []*Rule{{ID: "Dir/Max_Entries", Category: Directory, Check: noop}},
			want:  "malformed rule ID",
		},
		{
			name:  "unknown family",
			rules: []*Rule{{ID: "style/max-entries", Category: Directory, Check: noop}},
			want:  "malformed rule ID",
		},
		{
			name:  "category disagrees with prefix",
			rules: []*Rule{{ID: "org/member-order", Category: Directory, Check: noop}},
			want:  "does not match its ID prefix",
		},
		{
			name:  "nil check",
			rules: []*Rule{{ID: "dir/max-entries", Category: Directory}},
			want:  "no Check function",
		},
		{
			name: "duplicate ID",
			rules: []*Rule{
				{ID: "dir/max-entries", Category: Directory, Check: noop},
				{ID: "dir/max-entries", Category: Directory, Check: noop},
			},
			want: "registered twice",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSet(tt.rules)
			if err == nil {
				t.Fatalf("NewSet() = nil error, want one containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("NewSet() = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestInCategory(t *testing.T) {
	set, err := NewSet([]*Rule{
		{ID: "dir/max-entries", Category: Directory, Check: noop},
		{ID: "dir/domain-layout", Category: Directory, Check: noop},
		{ID: "pat/factory-naming", Category: Pattern, Check: noop},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := set.InCategory(Directory); len(got) != 2 {
		t.Errorf("InCategory(dir) returned %d rules, want 2", len(got))
	}
	if got := set.InCategory(Logic); len(got) != 0 {
		t.Errorf("InCategory(logic) returned %d rules, want 0", len(got))
	}
}

// TestEveryCategoryDescribes guards against a new family being added to the
// Category constants without a description, which `goorg rules` would print raw.
func TestEveryCategoryDescribes(t *testing.T) {
	for _, c := range Categories() {
		if got := c.Describe(); got == string(c) {
			t.Errorf("category %q has no description", c)
		}
	}
}

func TestTierString(t *testing.T) {
	if Syntax.String() != "syntax" || Types.String() != "types" {
		t.Errorf("tier names = %q, %q", Syntax, Types)
	}
}
