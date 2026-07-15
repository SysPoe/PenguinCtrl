package project

// AvailableBytes returns the bytes available to the current process on the
// filesystem containing path.
func AvailableBytes(path string) (uint64, error) {
	return cacheAvailableBytes(path)
}
