package main

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionOf(t *testing.T) {
	vcs := func(revision, modified string) *debug.BuildInfo {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: revision},
				{Key: "vcs.modified", Value: modified},
			},
		}
	}

	tests := []struct {
		name     string
		released string
		info     *debug.BuildInfo
		expected string
	}{
		{"release builds", "0.6.0", vcs("9851f89b5c3e4d2a", "false"), "0.6.0"},
		{"go install", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.6.0"}}, "v0.6.0"},
		{"local builds", "", vcs("9851f89b5c3e4d2a1f00", "false"), "(development build 9851f89b5c3e)"},
		{"local builds with changes", "", vcs("9851f89b5c3e4d2a1f00", "true"), "(development build 9851f89b5c3e-dirty)"},
		{"without vcs info", "", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "(development build)"},
		{"without build info", "", nil, "(development build)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, versionOf(tt.released, tt.info))
		})
	}
}
