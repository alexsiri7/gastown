package rig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureInstructionsSymlinks_CreatesGeminiMD(t *testing.T) {
	dir := t.TempDir()

	// Create AGENTS.md (the target file that should exist in worktrees).
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureInstructionsSymlinks(dir); err != nil {
		t.Fatalf("EnsureInstructionsSymlinks() error = %v", err)
	}

	// GEMINI.md should be created as a symlink to AGENTS.md.
	geminiPath := filepath.Join(dir, "GEMINI.md")
	target, err := os.Readlink(geminiPath)
	if err != nil {
		t.Fatalf("GEMINI.md symlink not created: %v", err)
	}
	if target != "AGENTS.md" {
		t.Errorf("GEMINI.md symlink target = %q, want %q", target, "AGENTS.md")
	}
}

func TestEnsureInstructionsSymlinks_NoAgentsMD(t *testing.T) {
	dir := t.TempDir()

	// No AGENTS.md — symlinks should not be created.
	if err := EnsureInstructionsSymlinks(dir); err != nil {
		t.Fatalf("EnsureInstructionsSymlinks() error = %v", err)
	}

	geminiPath := filepath.Join(dir, "GEMINI.md")
	if _, err := os.Lstat(geminiPath); !os.IsNotExist(err) {
		t.Errorf("GEMINI.md should not exist when AGENTS.md is missing")
	}
}

func TestEnsureInstructionsSymlinks_FixesBrokenSymlink(t *testing.T) {
	dir := t.TempDir()

	// Create AGENTS.md.
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a broken GEMINI.md symlink (town-root-relative path — the bug).
	geminiPath := filepath.Join(dir, "GEMINI.md")
	if err := os.Symlink("./reli/refinery/rig/AGENTS.md", geminiPath); err != nil {
		t.Fatal(err)
	}

	if err := EnsureInstructionsSymlinks(dir); err != nil {
		t.Fatalf("EnsureInstructionsSymlinks() error = %v", err)
	}

	// Should be fixed to point to AGENTS.md.
	target, err := os.Readlink(geminiPath)
	if err != nil {
		t.Fatalf("GEMINI.md symlink not found: %v", err)
	}
	if target != "AGENTS.md" {
		t.Errorf("GEMINI.md symlink target = %q, want %q", target, "AGENTS.md")
	}
}

func TestEnsureInstructionsSymlinks_SkipsRegularFile(t *testing.T) {
	dir := t.TempDir()

	// Create AGENTS.md.
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create GEMINI.md as a regular file (customer's file — don't overwrite).
	geminiPath := filepath.Join(dir, "GEMINI.md")
	if err := os.WriteFile(geminiPath, []byte("# Custom"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureInstructionsSymlinks(dir); err != nil {
		t.Fatalf("EnsureInstructionsSymlinks() error = %v", err)
	}

	// Should still be a regular file, not replaced.
	info, err := os.Lstat(geminiPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("GEMINI.md should remain a regular file, not be replaced with symlink")
	}
}

func TestEnsureInstructionsSymlinks_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run twice — second run should be a no-op.
	if err := EnsureInstructionsSymlinks(dir); err != nil {
		t.Fatalf("first call error = %v", err)
	}
	if err := EnsureInstructionsSymlinks(dir); err != nil {
		t.Fatalf("second call error = %v", err)
	}

	target, err := os.Readlink(filepath.Join(dir, "GEMINI.md"))
	if err != nil {
		t.Fatal(err)
	}
	if target != "AGENTS.md" {
		t.Errorf("GEMINI.md symlink target = %q, want %q", target, "AGENTS.md")
	}
}
