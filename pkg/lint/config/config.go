// Package config loads and resolves .goorg.yaml.
//
// The file answers three questions: which rules run and at what severity, what
// settings each rule gets, and which paths are off-limits. Anything that varies
// per invocation is a command-line flag instead, because this file is meant to
// be identical for every developer and for CI.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/glob"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// SchemaVersion is the only `version:` value this build understands. It exists
// so a future breaking change to the format fails loudly on an old binary
// rather than silently ignoring keys.
const SchemaVersion = 1

// FileNames are the config file names recognised in a project root, in
// preference order.
var FileNames = []string{".goorg.yaml", ".goorg.yml"}

// BuiltinExclude is always applied, ahead of any user configuration. testdata
// holds fixtures that are deliberately wrong; linting them would report the
// very violations they exist to reproduce.
var BuiltinExclude = []string{"**/testdata/**"}

// Config is the parsed contents of .goorg.yaml.
type Config struct {
	// Version must equal SchemaVersion.
	Version int `yaml:"version"`

	// Rules maps a rule ID, a category glob such as "dir/*", or "*" to a
	// severity. The most specific key wins regardless of the order it appears
	// in the file.
	Rules map[string]string `yaml:"rules"`

	// Settings holds each rule's own options, keyed by exact rule ID. Values
	// stay as raw YAML nodes until the rule that owns them decodes them, so
	// config never needs to know any rule's option struct.
	Settings map[string]yaml.Node `yaml:"settings"`

	// Exclude lists globs for paths goorg must not read. Patterns match
	// slash-separated root-relative paths and support *, ? and ** (any number
	// of segments). A pattern with no slash matches at any depth.
	Exclude []string `yaml:"exclude"`

	// Path is where this config was loaded from, or "" for defaults.
	Path string `yaml:"-"`
}

// Default returns the configuration used when a project has no .goorg.yaml:
// every rule at its own default severity.
func Default() *Config {
	return &Config{Version: SchemaVersion}
}

// Discover looks for a config file in dir. It returns "" with no error when
// there is none, since running without a config is a supported mode.
func Discover(dir string) (string, error) {
	for _, name := range FileNames {
		p := filepath.Join(dir, name)
		switch _, err := os.Stat(p); {
		case err == nil:
			return p, nil
		case !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("stat %s: %w", p, err)
		}
	}
	return "", nil
}

// Load reads and validates a config file against the active rule set.
func Load(path string, set *rule.Set) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{Path: path}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// Reject unknown keys. A misspelled field would otherwise mean a rule
	// silently never runs, which is the worst way for a linter to fail.
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(set); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks the config against the rule set, reporting every problem it
// finds rather than only the first — a bad config should take one round trip to
// fix, not five.
func (c *Config) Validate(set *rule.Set) error {
	var problems []string

	switch {
	case c.Version == 0:
		problems = append(problems, fmt.Sprintf("missing `version:`; add `version: %d`", SchemaVersion))
	case c.Version != SchemaVersion:
		problems = append(problems, fmt.Sprintf("unsupported version %d; this build understands version %d",
			c.Version, SchemaVersion))
	}

	for key, sev := range c.Rules {
		if _, err := diag.ParseSeverity(sev); err != nil {
			problems = append(problems, fmt.Sprintf("rules[%q]: %v", key, err))
		}
		if !matchesAnyRule(key, set) {
			problems = append(problems, fmt.Sprintf("rules[%q]: matches no known rule (see `goorg rules`)", key))
		}
	}

	for id := range c.Settings {
		if set.Get(id) == nil {
			problems = append(problems, fmt.Sprintf("settings[%q]: no such rule (see `goorg rules`)", id))
		}
	}

	for _, pattern := range c.Exclude {
		if strings.TrimSpace(pattern) == "" {
			problems = append(problems, "exclude: empty pattern")
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
}

// Severity resolves the configured severity for a rule.
//
// Specificity beats file order: an exact ID wins over a glob, and among globs
// the longer literal prefix wins, so `dir/*` overrides `*` and an exact ID
// overrides both.
func (c *Config) Severity(r *rule.Rule) diag.Severity {
	best, bestSpecificity := r.Default, -1
	for key, raw := range c.Rules {
		sev, err := diag.ParseSeverity(raw)
		if err != nil {
			continue // Validate already reported this.
		}
		spec := -1
		switch {
		case key == r.ID:
			spec = exactMatchSpecificity
		case strings.ContainsAny(key, "*?"):
			if glob.MatchRuleID(key, r.ID) {
				spec = len(strings.TrimRight(key, "*?"))
			}
		}
		if spec > bestSpecificity {
			best, bestSpecificity = sev, spec
		}
	}
	return best
}

// DecoderFor returns a closure that decodes a rule's settings onto a
// destination struct, or nil when the rule has no configured settings.
func (c *Config) DecoderFor(id string) func(any) error {
	node, ok := c.Settings[id]
	if !ok || node.IsZero() {
		return nil
	}
	return func(dst any) error {
		if err := node.Decode(dst); err != nil {
			return fmt.Errorf("settings[%q]: %w", id, err)
		}
		return nil
	}
}

// Excluder returns a predicate over root-relative slash paths, combining the
// built-in exclusions with the configured ones.
func (c *Config) Excluder() func(string) bool {
	patterns := make([]string, 0, len(BuiltinExclude)+len(c.Exclude))
	patterns = append(patterns, BuiltinExclude...)
	patterns = append(patterns, c.Exclude...)
	for i, p := range patterns {
		// A pattern with no separator applies at any depth, matching the
		// convention of .gitignore and every linter.
		p = strings.TrimSpace(p)
		if !strings.Contains(p, "/") {
			p = "**/" + p
		}
		patterns[i] = p
	}
	return func(rel string) bool {
		for _, p := range patterns {
			if glob.MatchPath(p, rel) {
				return true
			}
		}
		return false
	}
}

// exactMatchSpecificity is higher than any glob's literal prefix can be, so an
// exact rule ID always wins.
const exactMatchSpecificity = 1 << 20

// matchesAnyRule reports whether a config key names or globs at least one rule.
func matchesAnyRule(key string, set *rule.Set) bool {
	if set.Get(key) != nil {
		return true
	}
	if !strings.ContainsAny(key, "*?") {
		return false
	}
	for _, id := range set.IDs() {
		if glob.MatchRuleID(key, id) {
			return true
		}
	}
	return false
}
