// Package project is the application-facing facade for show document sessions.
// Durable recovery and cache storage policy live in internal subpackages; this
// package retains stable APIs and coordinates them with archive, library, and
// ProjectSession lifecycles.
package project
