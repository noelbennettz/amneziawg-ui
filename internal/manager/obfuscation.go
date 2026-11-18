package manager

import (
	"fmt"
	"strings"

	"amneziawg-web-ui/internal/awg"
	"amneziawg-web-ui/web-ui/api"
)

// generateObfuscationParams generates a full set of AmneziaWG 3.1 obfuscation
// parameters, including a header protection key. Obfuscation in this app is
// always AmneziaWG 3.x - there's no more separate 1.0/1.5/2.0 mode.
//
// The parameters themselves come from the shared generator, so an API caller
// that sends none gets the same shape the create form would have sent. Only
// the two fields it cannot know are filled in here: the MTU, and a key from a
// cryptographic source.
func generateObfuscationParams(mtu int) api.ObfuscationParams {
	p := api.GenerateObfuscation(mtu, false)
	p.MTU = mtu
	p.HeaderProtectionKey = awg.RandomKey()
	return *p
}

// validateObfuscationParams rejects a parameter set awg(8) or amneziawg-go
// would refuse later, when the failure would only show up as an interface
// that will not come up. The rules live in the shared api package, next to
// the type they describe, so the create form checks exactly these and the two
// sides cannot drift apart.
func validateObfuscationParams(p *api.ObfuscationParams, mtu int) error {
	if p == nil {
		return fmt.Errorf("obfuscation parameters are required")
	}
	if problems := p.Validate(mtu); len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// validateISettings checks a whole I1-I5 set against the grammar
// amneziawg-go parses in newObfChain. Without it a typo travels all the way
// into a client .conf and surfaces as a tunnel that will not start, on the
// user's machine rather than here. The grammar itself lives in the shared api
// package, so the client dialog rejects exactly the same values.
func validateISettings(settings api.ISettings) error {
	if problems := api.ValidateISettings(settings); len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}
