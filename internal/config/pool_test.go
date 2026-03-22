package config

import (
	"testing"
)

func TestRandomStrategyPick(t *testing.T) {
	s := &RandomStrategy{}
	agents := []string{"a", "b", "c"}
	// Run multiple times to verify it always returns a valid member
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		picked := s.Pick(agents)
		if picked != "a" && picked != "b" && picked != "c" {
			t.Fatalf("Pick returned %q, not in pool", picked)
		}
		seen[picked] = true
	}
	// With 100 iterations and 3 options, all should be seen (probability of missing one is vanishingly small)
	if len(seen) < 2 {
		t.Errorf("RandomStrategy appears non-random: only saw %v", seen)
	}
}

func TestResolvePoolConfig(t *testing.T) {
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"gemini-flash": {
				Command: "gemini",
				Args:    []string{"--model", "flash"},
			},
			"claude-sonnet": {
				Command: "claude",
				Args:    []string{"--model", "sonnet"},
			},
			"infra-pool": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"gemini-flash", "claude-sonnet"},
			},
		},
	}

	poolRC := townSettings.Agents["infra-pool"]

	// Resolve multiple times — should always return a valid concrete agent
	for i := 0; i < 20; i++ {
		rc, name, err := resolvePoolConfig(fillRuntimeDefaults(poolRC), townSettings, nil, 10)
		if err != nil {
			t.Fatalf("resolvePoolConfig failed: %v", err)
		}
		if name != "gemini-flash" && name != "claude-sonnet" {
			t.Errorf("unexpected picked name: %s", name)
		}
		if rc.Command != "gemini" && rc.Command != "claude" {
			t.Errorf("unexpected command: %s", rc.Command)
		}
		if rc.Type == "pool" {
			t.Error("resolved config should not be a pool")
		}
	}
}

func TestResolvePoolConfigDefaultStrategy(t *testing.T) {
	// When strategy is empty, should default to "random"
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"agent-a": {Command: "a"},
			"my-pool": {
				Type:       "pool",
				PoolAgents: []string{"agent-a"},
			},
		},
	}

	poolRC := fillRuntimeDefaults(townSettings.Agents["my-pool"])
	rc, name, err := resolvePoolConfig(poolRC, townSettings, nil, 10)
	if err != nil {
		t.Fatalf("resolvePoolConfig failed: %v", err)
	}
	if name != "agent-a" {
		t.Errorf("expected agent-a, got %s", name)
	}
	if rc.Command != "a" {
		t.Errorf("expected command 'a', got %s", rc.Command)
	}
}

func TestResolvePoolConfigUnknownStrategy(t *testing.T) {
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"agent-a": {Command: "a"},
			"my-pool": {
				Type:       "pool",
				Strategy:   "nonexistent",
				PoolAgents: []string{"agent-a"},
			},
		},
	}

	poolRC := fillRuntimeDefaults(townSettings.Agents["my-pool"])
	_, _, err := resolvePoolConfig(poolRC, townSettings, nil, 10)
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestResolvePoolConfigEmptyAgents(t *testing.T) {
	poolRC := &RuntimeConfig{
		Type:       "pool",
		Strategy:   "random",
		PoolAgents: []string{},
	}

	_, _, err := resolvePoolConfig(poolRC, &TownSettings{}, nil, 10)
	if err == nil {
		t.Fatal("expected error for empty pool agents")
	}
}

func TestResolvePoolConfigCircularReference(t *testing.T) {
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"pool-a": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"pool-b"},
			},
			"pool-b": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"pool-a"},
			},
		},
	}

	poolRC := fillRuntimeDefaults(townSettings.Agents["pool-a"])
	_, _, err := resolvePoolConfig(poolRC, townSettings, nil, 10)
	if err == nil {
		t.Fatal("expected error for circular pool reference")
	}
}

func TestResolvePoolConfigNestedPool(t *testing.T) {
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"agent-x":  {Command: "x"},
			"inner-pool": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"agent-x"},
			},
			"outer-pool": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"inner-pool"},
			},
		},
	}

	poolRC := fillRuntimeDefaults(townSettings.Agents["outer-pool"])
	rc, name, err := resolvePoolConfig(poolRC, townSettings, nil, 10)
	if err != nil {
		t.Fatalf("resolvePoolConfig failed: %v", err)
	}
	if name != "agent-x" {
		t.Errorf("expected agent-x, got %s", name)
	}
	if rc.Command != "x" {
		t.Errorf("expected command 'x', got %s", rc.Command)
	}
}

func TestResolvePoolConfigMissingMember(t *testing.T) {
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"my-pool": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"nonexistent-agent"},
			},
		},
	}

	poolRC := fillRuntimeDefaults(townSettings.Agents["my-pool"])
	_, _, err := resolvePoolConfig(poolRC, townSettings, nil, 10)
	if err == nil {
		t.Fatal("expected error for missing pool member")
	}
}

func TestIsPoolConfig(t *testing.T) {
	tests := []struct {
		name     string
		rc       *RuntimeConfig
		expected bool
	}{
		{"nil config", nil, false},
		{"empty type", &RuntimeConfig{}, false},
		{"direct type", &RuntimeConfig{Type: "direct"}, false},
		{"pool type", &RuntimeConfig{Type: "pool"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPoolConfig(tt.rc); got != tt.expected {
				t.Errorf("isPoolConfig() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPoolBackwardsCompatibility(t *testing.T) {
	// Existing configs without Type field should work unchanged
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"my-agent": {
				Command: "custom-cli",
				Args:    []string{"--flag"},
			},
		},
	}

	rc := lookupCustomAgentConfig("my-agent", townSettings, nil)
	if rc == nil {
		t.Fatal("expected non-nil config")
	}
	if isPoolConfig(rc) {
		t.Error("existing config without Type should not be a pool")
	}
	if rc.Command != "custom-cli" {
		t.Errorf("expected command 'custom-cli', got %s", rc.Command)
	}
}

func TestPoolResolutionInTryResolveNamedAgent(t *testing.T) {
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"agent-a": {Command: "agent-a-cmd"},
			"agent-b": {Command: "agent-b-cmd"},
			"test-pool": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"agent-a", "agent-b"},
			},
		},
	}

	// tryResolveNamedAgent should resolve the pool to a concrete agent
	for i := 0; i < 20; i++ {
		rc := tryResolveNamedAgent("test-pool", "test", townSettings, nil)
		if rc == nil {
			t.Fatal("expected non-nil config from pool resolution")
		}
		if rc.Type == "pool" {
			t.Error("tryResolveNamedAgent should have resolved the pool")
		}
		if rc.Command != "agent-a-cmd" && rc.Command != "agent-b-cmd" {
			t.Errorf("unexpected command: %s", rc.Command)
		}
		if rc.ResolvedAgent != "agent-a" && rc.ResolvedAgent != "agent-b" {
			t.Errorf("unexpected ResolvedAgent: %s", rc.ResolvedAgent)
		}
	}
}

func TestPoolWithRigSettings(t *testing.T) {
	// Pool defined in rig settings, members in town settings
	townSettings := &TownSettings{
		Agents: map[string]*RuntimeConfig{
			"global-agent": {Command: "global-cmd"},
		},
	}
	rigSettings := &RigSettings{
		Agents: map[string]*RuntimeConfig{
			"rig-pool": {
				Type:       "pool",
				Strategy:   "random",
				PoolAgents: []string{"global-agent"},
			},
		},
	}

	poolRC := fillRuntimeDefaults(rigSettings.Agents["rig-pool"])
	rc, name, err := resolvePoolConfig(poolRC, townSettings, rigSettings, 10)
	if err != nil {
		t.Fatalf("resolvePoolConfig failed: %v", err)
	}
	if name != "global-agent" {
		t.Errorf("expected global-agent, got %s", name)
	}
	if rc.Command != "global-cmd" {
		t.Errorf("expected command 'global-cmd', got %s", rc.Command)
	}
}
