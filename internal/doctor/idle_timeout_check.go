package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// IdleTimeoutCheck verifies that all rigs have dolt.idle-timeout set to "0"
// to prevent per-rig idle-monitors from spawning duplicate Dolt servers.
// Gas Town uses a centralized Dolt server managed by systemd.
type IdleTimeoutCheck struct {
	FixableCheck
}

// NewIdleTimeoutCheck creates a new idle timeout check.
func NewIdleTimeoutCheck() *IdleTimeoutCheck {
	return &IdleTimeoutCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "idle-timeout-config",
				CheckDescription: "Verify all rigs have dolt.idle-timeout set to \"0\" (centralized Dolt)",
				CheckCategory:    CategoryRig,
			},
		},
	}
}

// Run checks if all rigs and dog worktrees have dolt.idle-timeout set to "0".
func (c *IdleTimeoutCheck) Run(ctx *CheckContext) *CheckResult {
	// Load routes to get rig info
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routes, err := beads.LoadRoutes(townBeadsDir)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not load routes.jsonl",
		}
	}

	// Build unique rig list from routes
	rigSet := make(map[string]string) // rigName -> beadsPath
	for _, r := range routes {
		parts := strings.Split(r.Path, "/")
		if len(parts) >= 1 && parts[0] != "." {
			rigName := parts[0]
			if _, exists := rigSet[rigName]; !exists {
				rigSet[rigName] = r.Path
			}
		}
	}

	if len(rigSet) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No rigs to check",
		}
	}

	var missing []string
	var checked int

	// Check each rig for idle-timeout config
	for rigName, beadsPath := range rigSet {
		rigPath := filepath.Join(ctx.TownRoot, beadsPath)
		configPath := filepath.Join(rigPath, ".beads", "config.yaml")

		data, err := os.ReadFile(configPath)
		if err != nil {
			missing = append(missing, fmt.Sprintf("%s (config.yaml missing)", rigName))
			checked++
			continue
		}

		if !hasIdleTimeoutZero(string(data)) {
			missing = append(missing, rigName)
		}
		checked++
	}

	// Also check dog worktree .beads/ directories — these live outside the rig
	// tree and are not covered by routes. Dogs with missing or wrong idle-timeout
	// spawn rogue Dolt instances.
	dogMissing, dogChecked := checkDogWorktreeConfigs(ctx.TownRoot, rigSet)
	missing = append(missing, dogMissing...)
	checked += dogChecked

	if len(missing) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d locations have dolt.idle-timeout set to \"0\"", checked),
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d location(s) missing dolt.idle-timeout: \"0\"", len(missing)),
		Details: missing,
		FixHint: "Run 'gt doctor --fix' to add idle-timeout config to all rigs and dog worktrees",
	}
}

// hasIdleTimeoutZero returns true if the config content contains the correct
// dolt.idle-timeout: "0" setting.
func hasIdleTimeoutZero(content string) bool {
	return strings.Contains(content, "dolt.idle-timeout:") &&
		strings.Contains(content, `dolt.idle-timeout: "0"`)
}

// checkDogWorktreeConfigs scans deacon/dogs/<name>/<rig>/.beads/config.yaml
// for missing or incorrect idle-timeout settings.
func checkDogWorktreeConfigs(townRoot string, rigSet map[string]string) (missing []string, checked int) {
	dogsDir := filepath.Join(townRoot, "deacon", "dogs")
	dogEntries, err := os.ReadDir(dogsDir)
	if err != nil {
		return nil, 0 // No dogs directory — nothing to check
	}

	for _, dogEntry := range dogEntries {
		if !dogEntry.IsDir() {
			continue
		}
		dogName := dogEntry.Name()
		dogPath := filepath.Join(dogsDir, dogName)

		// Each dog has a subdirectory per rig
		for rigName := range rigSet {
			configPath := filepath.Join(dogPath, rigName, ".beads", "config.yaml")
			data, err := os.ReadFile(configPath)
			if err != nil {
				if os.IsNotExist(err) {
					// Check if the worktree directory exists at all
					if _, statErr := os.Stat(filepath.Join(dogPath, rigName)); statErr == nil {
						label := fmt.Sprintf("dog/%s/%s (config.yaml missing)", dogName, rigName)
						missing = append(missing, label)
						checked++
					}
				}
				continue
			}

			if !hasIdleTimeoutZero(string(data)) {
				missing = append(missing, fmt.Sprintf("dog/%s/%s", dogName, rigName))
			}
			checked++
		}
	}
	return missing, checked
}

// Fix adds dolt.idle-timeout: "0" to all rig and dog worktree config.yaml files.
func (c *IdleTimeoutCheck) Fix(ctx *CheckContext) error {
	// Load routes to get rig info
	townBeadsDir := filepath.Join(ctx.TownRoot, ".beads")
	routes, err := beads.LoadRoutes(townBeadsDir)
	if err != nil {
		return fmt.Errorf("loading routes.jsonl: %w", err)
	}

	// Build unique rig list from routes
	rigSet := make(map[string]string) // rigName -> beadsPath
	for _, r := range routes {
		parts := strings.Split(r.Path, "/")
		if len(parts) >= 1 && parts[0] != "." {
			rigName := parts[0]
			if _, exists := rigSet[rigName]; !exists {
				rigSet[rigName] = r.Path
			}
		}
	}

	// Fix each rig
	for rigName, beadsPath := range rigSet {
		rigPath := filepath.Join(ctx.TownRoot, beadsPath)
		rigBeadsPath := filepath.Join(rigPath, ".beads")

		if err := beads.EnsureConfigYAML(rigBeadsPath, ""); err != nil {
			return fmt.Errorf("fixing %s: %w", rigName, err)
		}
	}

	// Fix dog worktree .beads/ directories
	if err := fixDogWorktreeConfigs(ctx.TownRoot, rigSet); err != nil {
		return fmt.Errorf("fixing dog worktrees: %w", err)
	}

	return nil
}

// fixDogWorktreeConfigs ensures all dog worktree .beads/config.yaml files have
// the correct idle-timeout setting.
func fixDogWorktreeConfigs(townRoot string, rigSet map[string]string) error {
	dogsDir := filepath.Join(townRoot, "deacon", "dogs")
	dogEntries, err := os.ReadDir(dogsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No dogs directory
		}
		return err
	}

	for _, dogEntry := range dogEntries {
		if !dogEntry.IsDir() {
			continue
		}
		dogName := dogEntry.Name()
		dogPath := filepath.Join(dogsDir, dogName)

		for rigName := range rigSet {
			worktreeBeadsDir := filepath.Join(dogPath, rigName, ".beads")
			if _, err := os.Stat(worktreeBeadsDir); os.IsNotExist(err) {
				continue
			}

			prefix := beads.GetPrefixForRig(townRoot, rigName)
			if err := beads.EnsureConfigYAML(worktreeBeadsDir, prefix); err != nil {
				return fmt.Errorf("fixing dog/%s/%s: %w", dogName, rigName, err)
			}
		}
	}
	return nil
}
