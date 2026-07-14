// TODO(micro): Add a package comment and document Ter, or inline its only call and remove this one-function package.
package utils

func Ter[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}
