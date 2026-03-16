package sre

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Common errors
var (
	ErrNotRunning     = errors.New("sre not running")
	ErrAlreadyRunning = errors.New("sre already running")
)

// Manager handles SRE lifecycle and monitoring operations.
// ZFC-compliant: tmux session is the source of truth for running state.
type Manager struct {
	rig *rig.Rig
}

// NewManager creates a new SRE manager for a rig.
func NewManager(r *rig.Rig) *Manager {
	return &Manager{
		rig: r,
	}
}

// IsRunning checks if the SRE session is active and healthy.
// ZFC: tmux session existence is the source of truth for session state.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	status := t.CheckSessionHealth(m.SessionName(), 0)
	return status == tmux.SessionHealthy, nil
}

// IsHealthy checks if the SRE is running and has been active recently.
func (m *Manager) IsHealthy(maxInactivity time.Duration) tmux.ZombieStatus {
	t := tmux.NewTmux()
	return t.CheckSessionHealth(m.SessionName(), maxInactivity)
}

// SessionName returns the tmux session name for this SRE.
func (m *Manager) SessionName() string {
	return session.SRESessionName(session.PrefixFor(m.rig.Name))
}

// Status returns information about the SRE session.
// ZFC-compliant: tmux session is the source of truth.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	running, err := t.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return nil, ErrNotRunning
	}

	return t.GetSessionInfo(sessionID)
}

// sreDir returns the working directory for the SRE.
func (m *Manager) sreDir() string {
	sreDir := filepath.Join(m.rig.Path, "sre")
	if _, err := os.Stat(sreDir); err == nil {
		return sreDir
	}

	return m.rig.Path
}

// Start starts the SRE agent.
// If foreground is true, returns an error (foreground mode deprecated).
// Otherwise, spawns a Claude agent in a tmux session.
// ZFC-compliant: no state file, tmux session is source of truth.
func (m *Manager) Start(foreground bool, agentOverride string, envOverrides []string) error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	if foreground {
		return fmt.Errorf("foreground mode is deprecated; use background mode (remove --foreground flag)")
	}

	// Check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		if t.IsAgentAlive(sessionID) {
			return ErrAlreadyRunning
		}
		// Zombie detected — tmux alive but agent dead.
		createdAt, _ := t.GetSessionCreatedUnix(sessionID)
		time.Sleep(constants.ZombieKillGracePeriod)

		if t.IsAgentAlive(sessionID) {
			return ErrAlreadyRunning
		}
		if createdNow, _ := t.GetSessionCreatedUnix(sessionID); createdAt > 0 && createdNow != createdAt {
			return ErrAlreadyRunning
		}

		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Working directory
	sreDir := m.sreDir()

	// Ensure runtime settings exist.
	townRoot := m.townRoot()
	runtimeConfig := config.ResolveRoleAgentConfig("sre", townRoot, m.rig.Path)
	sreSettingsDir := config.RoleSettingsDir("sre", m.rig.Path)
	if err := runtime.EnsureSettingsForRole(sreSettingsDir, sreDir, "sre", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Ensure .gitignore has required Gas Town patterns
	if err := rig.EnsureGitignorePatterns(sreDir); err != nil {
		style.PrintWarning("could not update sre .gitignore: %v", err)
	}

	roleConfig, err := m.roleConfig()
	if err != nil {
		log.Printf("warning: could not load sre role config for %s: %v", m.rig.Name, err)
		roleConfig = nil
	}

	command, err := buildSREStartCommand(m.rig.Path, m.rig.Name, townRoot, sessionID, agentOverride, roleConfig)
	if err != nil {
		return err
	}

	runID := uuid.New().String()

	if err := t.NewSessionWithCommand(sessionID, sreDir, command); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:        "sre",
		Rig:         m.rig.Name,
		TownRoot:    townRoot,
		Agent:       agentOverride,
		SessionName: sessionID,
	})
	envVars = session.MergeRuntimeLivenessEnv(envVars, runtimeConfig)
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}
	_ = t.SetEnvironment(sessionID, "GT_RUN", runID)
	for key, value := range roleConfigEnvVars(roleConfig, townRoot, m.rig.Name) {
		if _, alreadySet := envVars[key]; alreadySet {
			continue
		}
		_ = t.SetEnvironment(sessionID, key, value)
	}
	for _, override := range envOverrides {
		if key, value, ok := strings.Cut(override, "="); ok {
			_ = t.SetEnvironment(sessionID, key, value)
		}
	}

	// Apply Gas Town theming
	theme := tmux.AssignTheme(m.rig.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, m.rig.Name, "sre", "sre")

	// Wait for Claude to start
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		_ = t.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("waiting for sre to start: %w", err)
	}

	if err := t.AcceptStartupDialogs(sessionID); err != nil {
		log.Printf("warning: accepting startup dialogs for %s: %v", sessionID, err)
	}

	if err := session.TrackSessionPID(townRoot, sessionID, t); err != nil {
		log.Printf("warning: tracking session PID for %s: %v", sessionID, err)
	}

	_ = runtime.RunStartupFallback(t, sessionID, "sre", runtimeConfig)
	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: session.BeaconRecipient("sre", "", m.rig.Name),
		Sender:    "deacon",
		Topic:     "patrol",
	}, "Run `gt prime --hook` and begin patrol.")
	_ = runtime.DeliverStartupPromptFallback(t, sessionID, initialPrompt, runtimeConfig, constants.ClaudeStartTimeout)

	// Stream JSONL conversation log to VictoriaLogs (opt-in).
	if os.Getenv("GT_LOG_AGENT_OUTPUT") == "true" && os.Getenv("GT_OTEL_LOGS_URL") != "" {
		if err := session.ActivateAgentLogging(sessionID, sreDir, runID); err != nil {
			log.Printf("warning: agent log watcher setup failed for %s: %v", sessionID, err)
		}
	}

	session.RecordAgentInstantiateFromDir(context.Background(), runID, runtimeConfig.ResolvedAgent,
		"sre", "sre", sessionID, m.rig.Name, townRoot, "", sreDir)

	time.Sleep(constants.ShutdownNotifyDelay)

	return nil
}

func (m *Manager) roleConfig() (*beads.RoleConfig, error) {
	townRoot := m.townRoot()
	roleDef, err := config.LoadRoleDefinition(townRoot, m.rig.Path, "sre")
	if err != nil {
		return nil, fmt.Errorf("loading sre role config: %w", err)
	}
	return &beads.RoleConfig{
		SessionPattern: roleDef.Session.Pattern,
		WorkDirPattern: roleDef.Session.WorkDir,
		NeedsPreSync:   roleDef.Session.NeedsPreSync,
		StartCommand:   roleDef.Session.StartCommand,
		EnvVars:        roleDef.Env,
	}, nil
}

func (m *Manager) townRoot() string {
	townRoot, err := workspace.Find(m.rig.Path)
	if err != nil || townRoot == "" {
		return m.rig.Path
	}
	return townRoot
}

func roleConfigEnvVars(roleConfig *beads.RoleConfig, townRoot, rigName string) map[string]string {
	if roleConfig == nil || len(roleConfig.EnvVars) == 0 {
		return nil
	}
	expanded := make(map[string]string, len(roleConfig.EnvVars))
	for key, value := range roleConfig.EnvVars {
		expanded[key] = beads.ExpandRolePattern(value, townRoot, rigName, "", "sre", session.PrefixFor(rigName))
	}
	return expanded
}

func buildSREStartCommand(rigPath, rigName, townRoot, sessionName, agentOverride string, roleConfig *beads.RoleConfig) (string, error) {
	if agentOverride != "" {
		roleConfig = nil
	}
	if roleConfig != nil && roleConfig.StartCommand != "" {
		rc := config.ResolveRoleAgentConfig("sre", townRoot, rigPath)
		if !config.IsResolvedAgentClaude(rc) || !isBuiltinClaudeStartCommand(roleConfig.StartCommand) {
			cmd := beads.ExpandRolePattern(roleConfig.StartCommand, townRoot, rigName, "", "sre", session.PrefixFor(rigName))
			if strings.HasPrefix(cmd, "exec ") {
				cmd = "exec env -u CLAUDECODE NODE_OPTIONS='' " + strings.TrimPrefix(cmd, "exec ")
			} else {
				cmd = "env -u CLAUDECODE NODE_OPTIONS='' " + cmd
			}
			return cmd, nil
		}
	}
	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: session.BeaconRecipient("sre", "", rigName),
		Sender:    "deacon",
		Topic:     "patrol",
	}, "Run `gt prime --hook` and begin patrol.")
	command, err := config.BuildStartupCommandFromConfig(config.AgentEnvConfig{
		Role:        "sre",
		Rig:         rigName,
		TownRoot:    townRoot,
		Prompt:      initialPrompt,
		Topic:       "patrol",
		SessionName: sessionName,
	}, rigPath, initialPrompt, agentOverride)
	if err != nil {
		return "", fmt.Errorf("building startup command: %w", err)
	}
	return command, nil
}

func isBuiltinClaudeStartCommand(cmd string) bool {
	trimmed := strings.TrimPrefix(cmd, "exec ")
	return trimmed == "claude --dangerously-skip-permissions"
}

// Stop stops the SRE agent.
// ZFC-compliant: tmux session is the source of truth.
func (m *Manager) Stop() error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	running, _ := t.HasSession(sessionID)
	if !running {
		return ErrNotRunning
	}

	return t.KillSession(sessionID)
}
