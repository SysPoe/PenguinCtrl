package redundancy

import "errors"

var errActiveCuesBlockHandoff = errors.New("STOP all local cues before changing command authority")

// AuthorityControl owns operator handoff policy around a redundancy Service.
// The composition root supplies playback activity and stop collaborators without
// coupling this package to the playback engine.
type AuthorityControl struct {
	service   *Service
	hasActive func() bool
	stopAll   func()
}

func NewAuthorityControl(service *Service, hasActive func() bool, stopAll func()) *AuthorityControl {
	if hasActive == nil {
		hasActive = func() bool { return false }
	}
	if stopAll == nil {
		stopAll = func() {}
	}
	return &AuthorityControl{service: service, hasActive: hasActive, stopAll: stopAll}
}

// Configure applies redundancy settings and stops local outputs if the node
// loses command authority while work is active. The result reports that stop.
func (c *AuthorityControl) Configure(config Config) (bool, error) {
	wasAuthority := c.service.Status().Authority
	if err := c.service.Configure(config); err != nil {
		return false, err
	}
	if wasAuthority && !c.service.Status().Authority && c.hasActive() {
		c.stopAll()
		return true, nil
	}
	return false, nil
}

func (c *AuthorityControl) RequestTakeover() error {
	if c.hasActive() {
		return errActiveCuesBlockHandoff
	}
	return c.service.RequestTakeover()
}

func (c *AuthorityControl) ReleaseAuthority() error {
	if c.hasActive() {
		return errActiveCuesBlockHandoff
	}
	return c.service.ReleaseAuthority()
}
