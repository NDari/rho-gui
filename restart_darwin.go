package main

import "os/exec"

// start opens the updated Rho.app, or runs the updated program when it
// is not in an app bundle.
func (p *restartPlan) start() error {
	if p.res.App != "" {
		return exec.Command("open", append([]string{"-n", p.res.App, "--args"}, p.args...)...).Start()
	}
	return exec.Command(p.res.Exe, p.args...).Start()
}
