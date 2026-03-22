package config

import (
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/constants"
)

func TestIsPoolConfig(t *testing.T) {
	tests := []struct {
		name string
		rc   *RuntimeConfig
		want bool
	}{
		{"nil", nil, false},
		{"empty type", &RuntimeConfig{}, false},
		{"agent type", &RuntimeConfig{Type: "agent"}, false},
		{"pool type", &RuntimeConfig{Type: "pool"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPoolConfig(tt.rc); got != tt.want {
				t.Errorf("IsPoolConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRandomStrategy(t *testing.T) {
	s := &RandomStrategy{}
	agents := []string{"a", "b", "c"}

	// Run enough times to verify it returns valid agents
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		picked := s.Pick(agents)
		found := false
		for _, a := range agents {
			if picked == a {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Pick returned %q which is not in agents list", picked)
		}
		seen[picked] = true
	}

	// With 100 trials and 3 agents, all should be seen
	if len(seen) != 3 {
		t.Errorf("expected all 3 agents to be picked, only saw %d", len(seen))
	}
}

func TestRandomStrategySingleAgent(t *testing.T) {
	s := &RandomStrategy{}
	got := s.Pick([]string{"only-one"})
	if got != "only-one" {
		t.Errorf("Pick() = %q, want %q", got, "only-one")
	}
}

func TestResolvePool(t *testing.T) {
	agents := map[string]*RuntimeConfig{
		"gemini-flash": {Command: "gemini", Args: []string{"--fast"}},
		"claude-sonnet": {Command: "claude", Args: []string{"--model", "sonnet"}},
	}

	pool := &RuntimeConfig{
		Type:       "pool",
		Strategy:   "random",
		PoolAgents: []string{"gemini-flash", "claude-sonnet"},
	}

	lookup := func(name string) *RuntimeConfig {
		return agents[name]
	}

	rc, picked, err := ResolvePool(pool, lookup)
	if err != nil {
		t.Fatalf("ResolvePool() error: %v", err)
	}
	if rc == nil {
		t.Fatal("ResolvePool() returned nil config")
	}
	if picked != "gemini-flash" && picked != "claude-sonnet" {
		t.Errorf("unexpected picked agent: %q", picked)
	}
	if rc.Command != agents[picked].Command {
		t.Errorf("resolved config command = %q, want %q", rc.Command, agents[picked].Command)
	}
}

func TestResolvePoolDefaultStrategy(t *testing.T) {
	agents := map[string]*RuntimeConfig{
		"a": {Command: "agent-a"},
	}
	pool := &RuntimeConfig{
		Type:       "pool",
		PoolAgents: []string{"a"},
		// Strategy empty — should default to "random"
	}

	rc, picked, err := ResolvePool(pool, func(name string) *RuntimeConfig {
		return agents[name]
	})
	if err != nil {
		t.Fatalf("ResolvePool() error: %v", err)
	}
	if picked != "a" {
		t.Errorf("picked = %q, want %q", picked, "a")
	}
	if rc.Command != "agent-a" {
		t.Errorf("command = %q, want %q", rc.Command, "agent-a")
	}
}

func TestResolvePoolNestedPools(t *testing.T) {
	agents := map[string]*RuntimeConfig{
		"inner-pool": {
			Type:       "pool",
			Strategy:   "random",
			PoolAgents: []string{"concrete"},
		},
		"concrete": {Command: "my-agent"},
	}
	outerPool := &RuntimeConfig{
		Type:       "pool",
		Strategy:   "random",
		PoolAgents: []string{"inner-pool"},
	}

	rc, picked, err := ResolvePool(outerPool, func(name string) *RuntimeConfig {
		return agents[name]
	})
	if err != nil {
		t.Fatalf("ResolvePool() error: %v", err)
	}
	if picked != "concrete" {
		t.Errorf("picked = %q, want %q", picked, "concrete")
	}
	if rc.Command != "my-agent" {
		t.Errorf("command = %q, want %q", rc.Command, "my-agent")
	}
}

func TestResolvePoolErrors(t *testing.T) {
	t.Run("not a pool", func(t *testing.T) {
		rc := &RuntimeConfig{Command: "claude"}
		_, _, err := ResolvePool(rc, func(string) *RuntimeConfig { return nil })
		if err == nil {
			t.Fatal("expected error for non-pool config")
		}
	})

	t.Run("empty pool", func(t *testing.T) {
		rc := &RuntimeConfig{Type: "pool", PoolAgents: []string{}}
		_, _, err := ResolvePool(rc, func(string) *RuntimeConfig { return nil })
		if err == nil {
			t.Fatal("expected error for empty pool")
		}
	})

	t.Run("unknown strategy", func(t *testing.T) {
		rc := &RuntimeConfig{
			Type:       "pool",
			Strategy:   "nonexistent",
			PoolAgents: []string{"a"},
		}
		_, _, err := ResolvePool(rc, func(string) *RuntimeConfig { return nil })
		if err == nil {
			t.Fatal("expected error for unknown strategy")
		}
	})

	t.Run("agent not found", func(t *testing.T) {
		rc := &RuntimeConfig{
			Type:       "pool",
			Strategy:   "random",
			PoolAgents: []string{"missing"},
		}
		_, _, err := ResolvePool(rc, func(string) *RuntimeConfig { return nil })
		if err == nil {
			t.Fatal("expected error for missing agent")
		}
	})

	t.Run("circular pool", func(t *testing.T) {
		agents := map[string]*RuntimeConfig{
			"pool-a": {Type: "pool", Strategy: "random", PoolAgents: []string{"pool-b"}},
			"pool-b": {Type: "pool", Strategy: "random", PoolAgents: []string{"pool-a"}},
		}
		_, _, err := ResolvePool(agents["pool-a"], func(name string) *RuntimeConfig {
			return agents[name]
		})
		if err == nil {
			t.Fatal("expected error for circular pool reference")
		}
	})
}

func TestRegisterPoolStrategy(t *testing.T) {
	// Register a deterministic strategy for testing
	RegisterPoolStrategy("first", &firstStrategy{})
	defer func() {
		// Cleanup
		poolStrategyMu.Lock()
		delete(poolStrategies, "first")
		poolStrategyMu.Unlock()
	}()

	s := GetPoolStrategy("first")
	if s == nil {
		t.Fatal("registered strategy not found")
	}

	got := s.Pick([]string{"a", "b", "c"})
	if got != "a" {
		t.Errorf("firstStrategy.Pick() = %q, want %q", got, "a")
	}
}

// firstStrategy always picks the first agent (for testing).
type firstStrategy struct{}

func (s *firstStrategy) Pick(agents []string) string { return agents[0] }

func TestFillRuntimeDefaultsPool(t *testing.T) {
	rc := &RuntimeConfig{
		Type:       "pool",
		Strategy:   "random",
		PoolAgents: []string{"a", "b"},
	}

	filled := fillRuntimeDefaults(rc)
	if filled.Type != "pool" {
		t.Errorf("Type = %q, want %q", filled.Type, "pool")
	}
	if filled.Strategy != "random" {
		t.Errorf("Strategy = %q, want %q", filled.Strategy, "random")
	}
	if len(filled.PoolAgents) != 2 {
		t.Errorf("PoolAgents len = %d, want 2", len(filled.PoolAgents))
	}
	// Should NOT have command defaults applied
	if filled.Command != "" {
		t.Errorf("pool should not have Command filled, got %q", filled.Command)
	}
}

func TestIsClaudeAgentPool(t *testing.T) {
	rc := &RuntimeConfig{Type: "pool"}
	if isClaudeAgent(rc) {
		t.Error("pool config should not be identified as Claude agent")
	}
}

func TestResolveRoleAgentConfig_PoolResolution(t *testing.T) {
	t.Parallel()
	ResetRegistryForTesting()
	defer ResetRegistryForTesting()

	townRoot := t.TempDir()
	rigPath := t.TempDir()

	// Create rig settings dir
	if err := os.MkdirAll(RigSettingsPath(rigPath)[:len(RigSettingsPath(rigPath))-len("/config.json")], 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Set up town settings with a pool that references two custom agents
	townSettings := NewTownSettings()
	townSettings.Agents = map[string]*RuntimeConfig{
		"agent-a": {Command: "claude", Args: []string{"--model", "sonnet"}},
		"agent-b": {Command: "claude", Args: []string{"--model", "haiku"}},
		"infra-pool": {
			Type:       "pool",
			Strategy:   "random",
			PoolAgents: []string{"agent-a", "agent-b"},
		},
	}
	townSettings.RoleAgents = map[string]string{
		constants.RoleRefinery: "infra-pool",
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create empty rig settings
	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// Resolve should return one of the pool's agents, not the pool itself
	rc := ResolveRoleAgentConfig(constants.RoleRefinery, townRoot, rigPath)
	if IsPoolConfig(rc) {
		t.Fatal("resolved config should not be a pool")
	}
	if rc.Command != "claude" {
		t.Errorf("Command = %q, want %q", rc.Command, "claude")
	}
	if rc.ResolvedAgent != "agent-a" && rc.ResolvedAgent != "agent-b" {
		t.Errorf("ResolvedAgent = %q, want agent-a or agent-b", rc.ResolvedAgent)
	}
}

func TestValidateAgentConfig_Pool(t *testing.T) {
	t.Parallel()
	ResetRegistryForTesting()
	defer ResetRegistryForTesting()

	townSettings := NewTownSettings()
	townSettings.Agents = map[string]*RuntimeConfig{
		"agent-a": {Command: "sh"}, // sh exists on all systems
		"good-pool": {
			Type:       "pool",
			Strategy:   "random",
			PoolAgents: []string{"agent-a"},
		},
		"empty-pool": {
			Type:       "pool",
			PoolAgents: []string{},
		},
		"bad-strategy-pool": {
			Type:       "pool",
			Strategy:   "nonexistent",
			PoolAgents: []string{"agent-a"},
		},
		"bad-member-pool": {
			Type:       "pool",
			Strategy:   "random",
			PoolAgents: []string{"missing-agent"},
		},
	}

	tests := []struct {
		name      string
		agent     string
		wantError bool
	}{
		{"valid pool", "good-pool", false},
		{"empty pool", "empty-pool", true},
		{"unknown strategy", "bad-strategy-pool", true},
		{"missing member", "bad-member-pool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentConfig(tt.agent, townSettings, nil)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateAgentConfig(%q) error = %v, wantError = %v", tt.agent, err, tt.wantError)
			}
		})
	}
}

func TestResolveRoleAgentConfig_BackwardsCompat(t *testing.T) {
	t.Parallel()
	ResetRegistryForTesting()
	defer ResetRegistryForTesting()

	townRoot := t.TempDir()

	// Plain agent config (no type field) should work exactly as before
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	rc := ResolveRoleAgentConfig("polecat", townRoot, "")
	if IsPoolConfig(rc) {
		t.Error("plain agent should not be a pool")
	}
	if !isClaudeAgent(rc) {
		t.Errorf("expected claude agent, got command=%q provider=%q", rc.Command, rc.Provider)
	}
}
