// Package buildinfo exposes release identity injected by the build pipeline.
package buildinfo

import "strings"

// These values are overridden by the release build's -ldflags. Defaults keep
// local developer builds identifiable without pretending they are releases.
var (
	// Version is the human-readable application version.
	Version = "dev"
	// Commit is the source revision used for this build.
	Commit = "unknown"
	// BuildTime is the release build timestamp.
	BuildTime = "unknown"
)

const shortCommitWidth = 12

// Identity returns the display version, including a shortened commit when the
// build pipeline supplied one.
func Identity() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}
	commit := strings.TrimSpace(Commit)
	if commit != "" && commit != "unknown" {
		if len(commit) > shortCommitWidth {
			commit = commit[:shortCommitWidth]
		}
		return version + "+" + commit
	}
	return version
}
