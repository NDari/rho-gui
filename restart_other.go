//go:build !darwin

package main

import "os/exec"

// start runs the updated program.
func (p *restartPlan) start() error {
	return exec.Command(p.res.Exe, p.args...).Start()
}
