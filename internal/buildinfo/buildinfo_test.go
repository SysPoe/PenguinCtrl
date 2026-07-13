package buildinfo

import "testing"

func TestIdentityIncludesBoundedCommit(t *testing.T) {
	oldVersion, oldCommit := Version, Commit
	t.Cleanup(func() { Version, Commit = oldVersion, oldCommit })
	Version, Commit = "v1.2.3", "1234567890abcdef"
	if got := Identity(); got != "v1.2.3+1234567890ab" {
		t.Fatalf("identity = %q", got)
	}
}
