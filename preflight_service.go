package main

import (
	"github.com/syspoe/cusus/preflight"
)

func newPreflightService() (*preflight.Service, error) {
	return preflight.NewService(readinessRefreshInterval, preflight.Assemble)
}
