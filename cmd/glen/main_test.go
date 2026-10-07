package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git-glen/internal/testutil"
	"git-glen/worktree"
)

func TestRunGlenCommand_Init(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Greenfield", func(t *testing.T) {
		emptyDir := testutil.SetupGreenfieldDirectory(t)
		var stdout, stderr bytes.Buffer

		code := RunGlenCommand(ctx, []string{"init", emptyDir}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
		}

		out := stdout.String()
		if !strings.Contains(out, "=== Pure Bare Repository Initialized ===") {
			t.Errorf("expected header in stdout, got: %s", out)
		}
		if !strings.Contains(out, "greenfield") {
			t.Errorf("expected greenfield scenario in stdout, got: %s", out)
		}
		if _, err := os.Stat(filepath.Join(emptyDir, ".git")); err != nil {
			t.Errorf("expected root .git pointer to exist: %v", err)
		}
	})
}

func TestRunGlenCommand_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rootDir, bareDir, _ := testutil.SetupPureBareWorkspace(t)

	// 1. Create a feature worktree with author identity
	var stdout, stderr bytes.Buffer
	code := RunGlenCommand(ctx, []string{
		"create", "feat/glen-test",
		"-C", rootDir,
		"--fetch=false",
		"--name", "Archer",
		"--email", "user+archer@example.com",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen create failed with code %d: %s", code, stderr.String())
	}

	expectedWT := filepath.Join(rootDir, "feat", "glen-test")
	if !strings.Contains(stdout.String(), expectedWT) {
		t.Errorf("expected stdout to contain %s, got: %s", expectedWT, stdout.String())
	}
	if _, err := os.Stat(expectedWT); err != nil {
		t.Fatalf("expected worktree directory %s to exist: %v", expectedWT, err)
	}

	gotName := testutil.RunGit(t, expectedWT, "config", "--get", "user.name")
	if gotName != "Archer" {
		t.Errorf("user.name = %q, want %q", gotName, "Archer")
	}

	// 2. List worktrees (JSON)
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"list", "-C", rootDir, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen list --json failed with code %d: %s", code, stderr.String())
	}
	var wts []worktree.WorktreeInfo
	if err := json.Unmarshal(stdout.Bytes(), &wts); err != nil {
		t.Fatalf("failed to unmarshal worktree list JSON: %v (raw: %s)", err, stdout.String())
	}
	if len(wts) != 3 {
		t.Errorf("expected 3 worktrees (.bare, main, feat/glen-test), got %d: %+v", len(wts), wts)
	}

	// 3. Remove worktree
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"remove", "feat/glen-test", "-C", rootDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen remove failed with code %d: %s", code, stderr.String())
	}
	if _, err := os.Stat(expectedWT); !os.IsNotExist(err) {
		t.Errorf("expected worktree directory %s to be removed", expectedWT)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "feat")); !os.IsNotExist(err) {
		t.Errorf("expected empty parent branch directory %s to be removed", filepath.Join(rootDir, "feat"))
	}

	branches := testutil.RunGit(t, bareDir, "branch", "--list", "feat/glen-test")
	if strings.TrimSpace(branches) != "" {
		t.Errorf("expected branch feat/glen-test to be deleted, got: %s", branches)
	}

	// 4. Prune worktrees
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"prune", "-C", rootDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen prune failed with code %d: %s", code, stderr.String())
	}
}
