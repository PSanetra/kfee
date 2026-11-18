package main

import (
	"runtime/debug"
)

// resolveVersion returns the version set by the release build, falling back to the
// build info Go embeds: the module version for `go install ...@version`, otherwise
// the git commit of local builds.
func resolveVersion() string {
	info, _ := debug.ReadBuildInfo()
	return versionOf(version, info)
}

func versionOf(released string, info *debug.BuildInfo) string {
	if released != "" {
		return released
	}
	if info == nil {
		return "(development build)"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if revision == "" {
		return "(development build)"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified == "true" {
		revision += "-dirty"
	}
	return "(development build " + revision + ")"
}
