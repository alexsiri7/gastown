package rig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// InstructionsSymlinkNames returns the filenames that EnsureInstructionsSymlinks
// would create. Used by localExcludePatterns to include them in local git exclude.
func InstructionsSymlinkNames() []string {
	var names []string
	for _, name := range config.ListAgentPresets() {
		preset := config.GetAgentPresetByName(name)
		if preset == nil || preset.InstructionsFile == "" {
			continue
		}
		agentMD := strings.ToUpper(string(preset.Name)) + ".md"
		if agentMD == preset.InstructionsFile || agentMD == "AGENTS.md" {
			continue
		}
		names = append(names, agentMD)
	}
	return names
}

// EnsureInstructionsSymlinks creates agent-specific instruction file symlinks
// in a worktree so that all supported agent runtimes can discover their
// instructions file by their conventional name.
//
// Pattern: <AGENT>.md → AGENTS.md (or CLAUDE.md for the claude preset)
//
// For example, Gemini CLI may look for GEMINI.md. This function creates
// GEMINI.md → AGENTS.md so the agent finds the project's instructions.
//
// At the town root, AGENTS.md → CLAUDE.md is created by install/upgrade.
// In worktrees, both CLAUDE.md and AGENTS.md are tracked files, so the
// symlinks created here point to whichever canonical file the agent uses.
//
// The symlinks use simple relative paths (e.g., "AGENTS.md") rather than
// town-root-relative paths, since they live in the worktree directory.
//
// Created symlinks are excluded from git via gasTownIgnorePatterns (applied
// by EnsureLocalExcludePatterns during worktree setup).
func EnsureInstructionsSymlinks(worktreePath string) error {
	for _, name := range config.ListAgentPresets() {
		preset := config.GetAgentPresetByName(name)
		if preset == nil || preset.InstructionsFile == "" {
			continue
		}

		// Derive the conventional agent instructions filename: <AGENT>.md
		// e.g., "gemini" → "GEMINI.md", "codex" → "CODEX.md"
		agentMD := strings.ToUpper(string(preset.Name)) + ".md"

		// Skip if the agent's conventional name IS its instructions file
		// (e.g., Claude uses CLAUDE.md which matches its agent name).
		if agentMD == preset.InstructionsFile {
			continue
		}

		// Skip agents whose name would collide with AGENTS.md.
		if agentMD == "AGENTS.md" {
			continue
		}

		linkPath := filepath.Join(worktreePath, agentMD)

		// Check if something already exists at the link path.
		if info, err := os.Lstat(linkPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				target, readErr := os.Readlink(linkPath)
				if readErr == nil && target == preset.InstructionsFile {
					continue // Already correct
				}
				// Wrong target — remove and recreate.
				if removeErr := os.Remove(linkPath); removeErr != nil {
					return fmt.Errorf("removing stale symlink %s: %w", agentMD, removeErr)
				}
			} else {
				// Regular file or directory — don't overwrite.
				continue
			}
		}

		// Only create symlink if the target file exists in the worktree.
		targetPath := filepath.Join(worktreePath, preset.InstructionsFile)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			continue
		}

		// Create worktree-relative symlink: <AGENT>.md → <InstructionsFile>
		if err := os.Symlink(preset.InstructionsFile, linkPath); err != nil {
			return fmt.Errorf("creating %s symlink: %w", agentMD, err)
		}
	}

	return nil
}
