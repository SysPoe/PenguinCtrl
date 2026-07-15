package preflight

import (
	"github.com/syspoe/cusus/operatorlog"
	"github.com/syspoe/cusus/show"
)

// Check describes one readiness observation presented before GO.
type Check struct {
	Severity     operatorlog.Severity
	Code         string
	Source       string
	Message      string
	Consequence  string
	Fix          string
	Field        string
	CueID        show.CueID
	CueNumber    string
	AffectedCues []show.CueID
	Fingerprint  string
	Acknowledged bool
}
