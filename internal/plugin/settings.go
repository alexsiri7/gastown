package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginSettings holds per-rig plugin enable/disable overrides.
// Stored at ~/gt/settings/plugins.json.
type PluginSettings struct {
	// Overrides maps rig names to plugin enable/disable overrides.
	// Each rig maps plugin names to their enabled state.
	Overrides map[string]map[string]bool `json:"overrides"`
}

// pluginSettingsPath returns the path to the plugin settings file.
func pluginSettingsPath(townRoot string) string {
	return filepath.Join(townRoot, "settings", "plugins.json")
}

// LoadPluginSettings loads plugin settings from settings/plugins.json.
// Returns empty settings (not an error) if the file doesn't exist.
func LoadPluginSettings(townRoot string) (*PluginSettings, error) {
	path := pluginSettingsPath(townRoot)
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		if os.IsNotExist(err) {
			return &PluginSettings{
				Overrides: make(map[string]map[string]bool),
			}, nil
		}
		return nil, fmt.Errorf("reading plugin settings: %w", err)
	}

	var settings PluginSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing plugin settings: %w", err)
	}
	if settings.Overrides == nil {
		settings.Overrides = make(map[string]map[string]bool)
	}
	return &settings, nil
}

// SavePluginSettings writes plugin settings to settings/plugins.json.
func SavePluginSettings(townRoot string, settings *PluginSettings) error {
	path := pluginSettingsPath(townRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding plugin settings: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: settings files don't contain secrets
		return fmt.Errorf("writing plugin settings: %w", err)
	}
	return nil
}

// IsEnabled resolves whether a plugin is enabled for a given rig.
// Resolution order:
//  1. Per-rig override in settings/plugins.json
//  2. Frontmatter enabled field
//  3. Default: true
func (s *PluginSettings) IsEnabled(pluginName, rigName string, frontmatterEnabled bool) bool {
	if rigOverrides, ok := s.Overrides[rigName]; ok {
		if enabled, ok := rigOverrides[pluginName]; ok {
			return enabled
		}
	}
	return frontmatterEnabled
}

// SetOverride sets an enable/disable override for a plugin in a rig.
func (s *PluginSettings) SetOverride(rigName, pluginName string, enabled bool) {
	if s.Overrides[rigName] == nil {
		s.Overrides[rigName] = make(map[string]bool)
	}
	s.Overrides[rigName][pluginName] = enabled
}

// ClearOverride removes an override for a plugin in a rig,
// reverting to the frontmatter default.
func (s *PluginSettings) ClearOverride(rigName, pluginName string) {
	if rigOverrides, ok := s.Overrides[rigName]; ok {
		delete(rigOverrides, pluginName)
		if len(rigOverrides) == 0 {
			delete(s.Overrides, rigName)
		}
	}
}
