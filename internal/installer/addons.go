package installer

import "fmt"

// The pieces that can be added to a machine that is already running, one at a time. Each is the step a fresh install would run for it,
// so the result is the same as if it had been asked for at the start.

// PHPPlan installs one PHP version (o.PHPVersions) with its FPM service, and leaves the service stopped until a site uses it.
func PHPPlan(o Options) (Plan, error) {
	if len(o.PHPVersions) != 1 {
		return nil, fmt.Errorf("one PHP version at a time")
	}
	s, err := phpStep(o)
	if err != nil {
		return nil, err
	}
	return Plan{s}, nil
}

// NodePlan installs one Node.js major version under /opt/node.
func NodePlan(major string) (Plan, error) {
	s, err := nodeStep(major)
	if err != nil {
		return nil, err
	}
	return Plan{s}, nil
}

// DatabasePlan installs a database server (o.Engine) with the default series for the engine and the settings that fit the machine. o
// needs Target, Profile and the PHP versions that are installed.
func DatabasePlan(o Options) (Plan, error) {
	s, err := database(o)
	if err != nil {
		return nil, err
	}
	return Plan{s}, nil
}
