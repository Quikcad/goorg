package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// resolveRoot finds the project root: the nearest ancestor of dir holding a
// go.mod or a goorg config. Falling back to dir itself means goorg still works
// in a directory that is not a module, which matters when checking one
// subdirectory of a monorepo.
func resolveRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	for cur := abs; ; {
		if hasProjectMarker(cur) {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		cur = parent
	}
}

func hasProjectMarker(dir string) bool {
	names := append([]string{"go.mod"}, config.FileNames()...)
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// loadConfig resolves the configuration for a root, honoring an explicit
// --config path when one is given.
func loadConfig(root, explicit string, set *rule.Set) (*config.Config, error) {
	if explicit != "" {
		return config.Load(explicit, set)
	}
	found, err := config.Discover(root)
	if err != nil {
		return nil, err
	}
	if found == "" {
		return config.Default(), nil
	}
	return config.Load(found, set)
}

// normalizePaths turns Go-style package patterns into directories relative to
// root. goorg is always recursive, so `./...` and `.` mean the same thing;
// accepting both is about matching muscle memory, not semantics.
func normalizePaths(root string, args []string) ([]string, error) {
	var out []string
	for _, arg := range args {
		p := strings.TrimSuffix(strings.TrimSuffix(filepath.ToSlash(arg), "..."), "/")
		if p == "" || p == "." {
			// The whole tree: no filtering needed, so drop every other path.
			return nil, nil
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", arg, err)
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, fmt.Errorf("resolve %s against project root %s: %w", arg, root, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return nil, fmt.Errorf("path %s is outside the project root %s", arg, root)
		}
		if rel == "." {
			return nil, nil
		}
		out = append(out, rel)
	}
	sort.Strings(out)
	return out, nil
}
