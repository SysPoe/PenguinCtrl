package project

import projectcache "github.com/syspoe/cusus/project/internal/cache"

// AvailableBytes returns the bytes available to the current process on the
// filesystem containing path.
func AvailableBytes(path string) (uint64, error) {
	return projectcache.AvailableBytes(path)
}
