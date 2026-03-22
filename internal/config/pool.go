// Package config provides configuration types and serialization for Gas Town.
package config

import (
	"fmt"
	"math/rand/v2"
	"sync"
)

// PoolStrategy selects a concrete agent from a pool of agent names.
// Implementations must be safe for concurrent use.
type PoolStrategy interface {
	// Pick selects one agent name from the given list.
	// agents is guaranteed to be non-empty.
	Pick(agents []string) string
}

// RandomStrategy picks a random agent from the pool.
type RandomStrategy struct{}

// Pick returns a uniformly random agent from the pool.
func (s *RandomStrategy) Pick(agents []string) string {
	return agents[rand.IntN(len(agents))]
}

// Pool strategy registry.
var (
	poolStrategyMu   sync.RWMutex
	poolStrategies   = map[string]PoolStrategy{
		"random": &RandomStrategy{},
	}
)

// RegisterPoolStrategy registers a named pool strategy.
// Overwrites any existing strategy with the same name.
func RegisterPoolStrategy(name string, strategy PoolStrategy) {
	poolStrategyMu.Lock()
	defer poolStrategyMu.Unlock()
	poolStrategies[name] = strategy
}

// GetPoolStrategy returns the pool strategy for the given name.
// Returns nil if the strategy is not registered.
func GetPoolStrategy(name string) PoolStrategy {
	poolStrategyMu.RLock()
	defer poolStrategyMu.RUnlock()
	return poolStrategies[name]
}

// IsPoolConfig returns true if the RuntimeConfig represents a pool.
func IsPoolConfig(rc *RuntimeConfig) bool {
	return rc != nil && rc.Type == "pool"
}

// maxPoolDepth limits recursive pool resolution to prevent infinite loops.
const maxPoolDepth = 10

// ResolvePool picks a concrete agent from a pool config and returns its
// RuntimeConfig. It resolves recursively if the picked agent is itself a pool.
//
// The lookup function maps agent names to their RuntimeConfig. It should check
// rig agents, town agents, and built-in presets (same as lookupAgentConfig).
//
// Returns an error if:
//   - rc is not a pool config
//   - the pool has no agents
//   - the strategy is unknown
//   - recursive resolution exceeds maxPoolDepth
//   - a picked agent cannot be found
func ResolvePool(rc *RuntimeConfig, lookup func(name string) *RuntimeConfig) (*RuntimeConfig, string, error) {
	return resolvePoolRecursive(rc, lookup, 0)
}

func resolvePoolRecursive(rc *RuntimeConfig, lookup func(name string) *RuntimeConfig, depth int) (*RuntimeConfig, string, error) {
	if depth > maxPoolDepth {
		return nil, "", fmt.Errorf("pool resolution exceeded max depth %d (circular pool reference?)", maxPoolDepth)
	}

	if !IsPoolConfig(rc) {
		return nil, "", fmt.Errorf("not a pool config (type=%q)", rc.Type)
	}

	if len(rc.PoolAgents) == 0 {
		return nil, "", fmt.Errorf("pool has no agents")
	}

	strategyName := rc.Strategy
	if strategyName == "" {
		strategyName = "random"
	}

	strategy := GetPoolStrategy(strategyName)
	if strategy == nil {
		return nil, "", fmt.Errorf("unknown pool strategy %q", strategyName)
	}

	picked := strategy.Pick(rc.PoolAgents)

	resolved := lookup(picked)
	if resolved == nil {
		return nil, "", fmt.Errorf("pool agent %q not found", picked)
	}

	// If the picked agent is itself a pool, resolve recursively.
	if IsPoolConfig(resolved) {
		return resolvePoolRecursive(resolved, lookup, depth+1)
	}

	return resolved, picked, nil
}
