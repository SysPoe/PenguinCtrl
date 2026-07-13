//go:build windows

package media

import "testing"

func TestEnumerateVideoDisplaysReusesCallback(t *testing.T) {
	// Go's Windows callback table holds roughly 2,000 entries and callbacks cannot
	// be released. A callback allocated on every refresh used to crash the app
	// after about 66 minutes with the two-second display polling interval.
	for range 2100 {
		if _, err := enumerateVideoDisplays(); err != nil {
			t.Fatalf("enumerate video displays: %v", err)
		}
	}
}
