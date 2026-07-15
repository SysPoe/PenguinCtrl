// Package utils provides small generic helpers shared by UI packages.
package utils

// Ter returns a when cond is true and b otherwise. Both values are evaluated
// before Ter is called; use an if statement when either branch is expensive or
// has side effects.
func Ter[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
