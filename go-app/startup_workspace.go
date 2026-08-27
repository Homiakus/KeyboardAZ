package main

import (
	"fmt"
	"log"

	"hapticpad-go-app/workspace"
	"hapticpad-go-app/workspacemigrate"
)

// prepareWorkspace resolves the canonical per-user workspace and performs a
// validated, non-destructive migration from the legacy ~/.hapticpad location.
// A resolution failure falls back to the legacy location so startup remains
// available instead of silently switching to an arbitrary working directory.
func prepareWorkspace() (workspace.Paths, string) {
	paths, err := workspace.Default()
	if err != nil {
		legacyRoot, legacyErr := workspace.LegacyRoot()
		if legacyErr != nil {
			return workspace.Paths{}, fmt.Sprintf("Workspace resolution failed: %v · legacy fallback failed: %v", err, legacyErr)
		}
		paths = workspace.FromRoot(legacyRoot)
		startupError := fmt.Sprintf("Workspace resolution failed, using legacy path: %v", err)
		if ensureErr := paths.Ensure(); ensureErr != nil {
			startupError = appendStartupError(startupError, fmt.Sprintf("Workspace initialization failed: %v", ensureErr))
		}
		return paths, startupError
	}

	startupError := ""
	if err := paths.Ensure(); err != nil {
		startupError = appendStartupError(startupError, fmt.Sprintf("Workspace initialization failed: %v", err))
	}

	legacyRoot, legacyErr := workspace.LegacyRoot()
	if legacyErr != nil {
		log.Printf("Legacy workspace lookup skipped: %v", legacyErr)
		return paths, startupError
	}

	result, migrationErr := workspacemigrate.MigrateValidated(workspace.FromRoot(legacyRoot), paths)
	if migrationErr != nil {
		startupError = appendStartupError(startupError, fmt.Sprintf("Legacy workspace migration incomplete: %v", migrationErr))
	}
	if result.LayoutCopied || result.KeymapCopied || result.DeviceIdentityCopied {
		log.Printf(
			"Legacy workspace migrated to %s (layout=%t keymap=%t device=%t); source preserved for rollback",
			paths.Root,
			result.LayoutCopied,
			result.KeymapCopied,
			result.DeviceIdentityCopied,
		)
	}
	return paths, startupError
}

func canonicalConfigDir() string {
	paths, err := workspace.Default()
	if err == nil {
		return paths.Root
	}
	legacyRoot, legacyErr := workspace.LegacyRoot()
	if legacyErr == nil {
		return legacyRoot
	}
	return "."
}

func appendStartupError(current, next string) string {
	if next == "" {
		return current
	}
	if current == "" {
		return next
	}
	return current + " · " + next
}
