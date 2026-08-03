package main

// shouldHideToTray keeps the policy independent from Win32 code so it can be
// regression-tested on every platform.
func shouldHideToTray(minimized, exiting bool) bool {
	return minimized && !exiting
}
