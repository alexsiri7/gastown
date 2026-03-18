package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginSettings_IsEnabled(t *testing.T) {
	tests := []struct {
		name               string
		overrides          map[string]map[string]bool
		pluginName         string
		rigName            string
		frontmatterEnabled bool
		want               bool
	}{
		{
			name:               "default true when no override",
			overrides:          map[string]map[string]bool{},
			pluginName:         "test-plugin",
			rigName:            "myrig",
			frontmatterEnabled: true,
			want:               true,
		},
		{
			name:               "frontmatter disabled, no override",
			overrides:          map[string]map[string]bool{},
			pluginName:         "test-plugin",
			rigName:            "myrig",
			frontmatterEnabled: false,
			want:               false,
		},
		{
			name: "override enables a disabled plugin",
			overrides: map[string]map[string]bool{
				"myrig": {"test-plugin": true},
			},
			pluginName:         "test-plugin",
			rigName:            "myrig",
			frontmatterEnabled: false,
			want:               true,
		},
		{
			name: "override disables an enabled plugin",
			overrides: map[string]map[string]bool{
				"myrig": {"test-plugin": false},
			},
			pluginName:         "test-plugin",
			rigName:            "myrig",
			frontmatterEnabled: true,
			want:               false,
		},
		{
			name: "override for different rig does not apply",
			overrides: map[string]map[string]bool{
				"otherrig": {"test-plugin": false},
			},
			pluginName:         "test-plugin",
			rigName:            "myrig",
			frontmatterEnabled: true,
			want:               true,
		},
		{
			name: "override for different plugin does not apply",
			overrides: map[string]map[string]bool{
				"myrig": {"other-plugin": false},
			},
			pluginName:         "test-plugin",
			rigName:            "myrig",
			frontmatterEnabled: true,
			want:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &PluginSettings{Overrides: tt.overrides}
			got := s.IsEnabled(tt.pluginName, tt.rigName, tt.frontmatterEnabled)
			if got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPluginSettings_SetAndClearOverride(t *testing.T) {
	s := &PluginSettings{Overrides: make(map[string]map[string]bool)}

	// Set override
	s.SetOverride("myrig", "test-plugin", false)
	if s.IsEnabled("test-plugin", "myrig", true) {
		t.Error("expected plugin to be disabled after SetOverride(false)")
	}

	// Clear override
	s.ClearOverride("myrig", "test-plugin")
	if !s.IsEnabled("test-plugin", "myrig", true) {
		t.Error("expected plugin to be enabled after ClearOverride (frontmatter default)")
	}

	// Verify rig map is cleaned up
	if _, ok := s.Overrides["myrig"]; ok {
		t.Error("expected rig map to be removed when empty")
	}
}

func TestLoadSavePluginSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Load from non-existent file returns empty settings.
	settings, err := LoadPluginSettings(tmpDir)
	if err != nil {
		t.Fatalf("LoadPluginSettings() error = %v", err)
	}
	if len(settings.Overrides) != 0 {
		t.Errorf("expected empty overrides, got %v", settings.Overrides)
	}

	// Save and reload.
	settings.SetOverride("reli", "github-sheriff", true)
	settings.SetOverride("reli", "quality-review", false)
	settings.SetOverride("annie", "github-sheriff", true)

	if err := SavePluginSettings(tmpDir, settings); err != nil {
		t.Fatalf("SavePluginSettings() error = %v", err)
	}

	// Verify file exists.
	path := filepath.Join(tmpDir, "settings", "plugins.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file not created: %v", err)
	}

	// Reload and verify.
	loaded, err := LoadPluginSettings(tmpDir)
	if err != nil {
		t.Fatalf("LoadPluginSettings() after save error = %v", err)
	}

	if !loaded.IsEnabled("github-sheriff", "reli", false) {
		t.Error("expected github-sheriff enabled for reli")
	}
	if loaded.IsEnabled("quality-review", "reli", true) {
		t.Error("expected quality-review disabled for reli")
	}
	if !loaded.IsEnabled("github-sheriff", "annie", false) {
		t.Error("expected github-sheriff enabled for annie")
	}
}
