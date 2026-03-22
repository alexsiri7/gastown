package config

import (
	"fmt"
	"math/rand"
)

// PoolStrategy selects one agent name from a pool of candidates.
type PoolStrategy interface {
	Pick(agents []string) string
}

// poolStrategies is the pluggable registry of pool strategies.
var poolStrategies = map[string]PoolStrategy{
	"random": &RandomStrategy{},
}

// RandomStrategy picks a random agent from the pool.
type RandomStrategy struct{}

func (s *RandomStrategy) Pick(agents []string) string {
	return agents[rand.Intn(len(agents))]
}

// resolvePoolConfig resolves a pool RuntimeConfig to a concrete agent config.
// It picks an agent from the pool using the configured strategy, then looks up
// that agent's config. Resolution is recursive — a pool member can itself be a pool.
//
// maxDepth prevents infinite loops from circular pool references.
func resolvePoolConfig(poolRC *RuntimeConfig, townSettings *TownSettings, rigSettings *RigSettings, maxDepth int) (*RuntimeConfig, string, error) {
	if maxDepth <= 0 {
		return nil, "", fmt.Errorf("pool resolution exceeded max depth (circular pool reference?)")
	}

	if len(poolRC.PoolAgents) == 0 {
		return nil, "", fmt.Errorf("pool has no agents to pick from")
	}

	strategyName := poolRC.Strategy
	if strategyName == "" {
		strategyName = "random"
	}

	strategy, ok := poolStrategies[strategyName]
	if !ok {
		return nil, "", fmt.Errorf("unknown pool strategy: %q", strategyName)
	}

	picked := strategy.Pick(poolRC.PoolAgents)

	// Look up the picked agent
	rc := lookupAgentConfigIfExists(picked, townSettings, rigSettings)
	if rc == nil {
		return nil, "", fmt.Errorf("pool member %q not found as agent", picked)
	}

	// Recursive resolution if the picked agent is also a pool
	if rc.Type == "pool" {
		return resolvePoolConfig(rc, townSettings, rigSettings, maxDepth-1)
	}

	return rc, picked, nil
}

// isPoolConfig returns true if the RuntimeConfig represents a pool agent.
func isPoolConfig(rc *RuntimeConfig) bool {
	return rc != nil && rc.Type == "pool"
}
