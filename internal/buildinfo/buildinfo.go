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

// TODO(macro): Config/Show schema versions are serialization contracts for the
// config and show packages, not build identity. They are unused here; own them
// next to the formats that enforce them (or a versioning package) so release
// ldflags/Identity stay decoupled from document schema bumps.
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
		// TODO(micro): name the short-hash width (12) as a constant
		if len(commit) > 12 {
			commit = commit[:12]
		}
		return version + "+" + commit
	}
	return version
}
