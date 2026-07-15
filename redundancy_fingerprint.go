package main

import (
	"github.com/syspoe/cusus/config"
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/preflight"
	"github.com/syspoe/cusus/project"
	"github.com/syspoe/cusus/redundancy"
	"github.com/syspoe/cusus/show"
)

func updateRedundancyFingerprint(service *redundancy.Service, current show.Show, settings config.Settings, files []project.File, preflightReady bool, previousError string, report func(string)) string {
	fingerprint, err := redundancy.BuildFingerprint(current, settings, files, preflightReady)
	if err == nil {
		service.UpdateFingerprint(fingerprint)
		return ""
	}
	service.UpdateFingerprint(redundancy.Fingerprint{})
	if message := err.Error(); message != previousError {
		if report != nil {
			report(message)
		}
		return message
	}
	return previousError
}

func redundancyPreflightReady(checks []preflight.Check) bool {
	for _, check := range checks {
		if check.Severity == operatorlog.ShowStopping {
			return false
		}
	}
	return true
}
