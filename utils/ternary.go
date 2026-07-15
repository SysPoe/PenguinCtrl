// TODO(micro): Add a package comment and document Ter, or inline its only call and remove this one-function package.
package utils

// TODO(micro): Ter eagerly evaluates both a and b (value args); for expensive branches use if/else or pass funcs — document non-short-circuiting semantics or rename to Choose
func Ter[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
