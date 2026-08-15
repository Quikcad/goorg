// Package buildinfo exposes the version stamped into the binary at link time,
// falling back to what the Go toolchain recorded.
package buildinfo

import "runtime/debug"

// version is overridden at build time via:
//
//	-ldflags '-X github.com/Quikcad/goorg/internal/buildinfo.version=v1.2.3'
//
//goorg:ignore org/globals-singleton-only — -X can only target a package-level string
var version = ""

// Version returns the best available description of this build: the link-time
// stamp, then the version embedded by `go install module@version`, then the VCS
// revision, and finally "dev".
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	rev := setting(info, "vcs.revision")
	if rev == "" {
		return "dev"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if setting(info, "vcs.modified") == "true" {
		return rev + "-dirty"
	}
	return rev
}

func setting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
