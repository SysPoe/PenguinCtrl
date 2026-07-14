// TODO(micro): Add a package comment and Go-style docs for the exported schema constant and Identity function.
package buildinfo

import "strings"

// These values are overridden by the release build's -ldflags. Defaults keep
// local developer builds identifiable without pretending they are releases.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

const (
	ConfigSchemaVersion = 1
	ShowSchemaVersion   = 2
)

func Identity() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		version = "dev"
	}
	commit := strings.TrimSpace(Commit)
	if commit != "" && commit != "unknown" {
		if len(commit) > 12 {
			commit = commit[:12]
		}
		return version + "+" + commit
	}
	return version
}
