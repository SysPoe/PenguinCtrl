// Package osc provides shared OSC string framing primitives.
package osc

import (
	"errors"
	"strings"
)

const alignment = 4

// AppendString appends a null-terminated, four-byte-aligned OSC string.
func AppendString(destination []byte, value string) ([]byte, error) {
	if strings.ContainsRune(value, 0) {
		return nil, errors.New("OSC strings cannot contain NUL")
	}
	destination = append(destination, value...)
	destination = append(destination, 0)
	for len(destination)%alignment != 0 {
		destination = append(destination, 0)
	}
	return destination, nil
}

// ReadString decodes an OSC string at offset and returns the next aligned
// offset.
func ReadString(packet []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(packet) {
		return "", 0, errors.New("short OSC string")
	}
	end := offset
	for end < len(packet) && packet[end] != 0 {
		end++
	}
	if end == len(packet) {
		return "", 0, errors.New("unterminated OSC string")
	}
	next := (end + alignment) &^ (alignment - 1)
	if next > len(packet) {
		return "", 0, errors.New("short OSC padding")
	}
	return string(packet[offset:end]), next, nil
}
