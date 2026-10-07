package testutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git-glen/internal/testutil"
)

func TestWorkspaceFixtures(t *testing.T) {
	t.Run("SetupPureBareWorkspace", func(t *testing.T) {
		rootDir, bareDir, mainDir := testutil.SetupPureBareWorkspace(t)

		if _, err := os.Stat(bareDir); err != nil {
			t.Errorf("expected .bare directory to exist at %s: %v", bareDir, err)
		}
		if _, err := os.Stat(filepath.Join(rootDir, ".git")); err != nil {
			t.Errorf("expected root .git pointer to exist at %s: %v", filepath.Join(rootDir, ".git"), err)
		}
		if _, err := os.Stat(mainDir); err != nil {
			t.Errorf("expected main worktree directory to exist at %s: %v", mainDir, err)
		}

		// Verify main worktree git status is clean
		out := testutil.RunGit(t, mainDir, "status", "--porcelain")
		if out != "" {
			t.Errorf("expected clean working tree in main, got: %s", out)
		}
	})

	t.Run("SetupStandardClone_Clean", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		gitDir := filepath.Join(cloneDir, ".git")
		if _, err := os.Stat(gitDir); err != nil {
			t.Errorf("expected .git directory to exist at %s: %v", gitDir, err)
		}

		out := testutil.RunGit(t, cloneDir, "status", "--porcelain")
		if out != "" {
			t.Errorf("expected clean clone, got status: %s", out)
		}
	})

	t.Run("SetupStandardClone_Dirty", func(t *testing.T) {
		dirtyClone := testutil.SetupDirtyClone(t)
		statusOut := testutil.RunGit(t, dirtyClone, "status", "--porcelain")
		if !strings.Contains(statusOut, "scratch.txt") || !strings.Contains(statusOut, "README.md") {
			t.Errorf("status --porcelain = %q, want untracked scratch.txt and modified README.md", statusOut)
		}
	})

	t.Run("SetupLegacySiblingLayout", func(t *testing.T) {
		rootDir, repoDir, worktreesDir := testutil.SetupLegacySiblingLayout(t)

		if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
			t.Errorf("expected legacy repo/.git to exist: %v", err)
		}
		if _, err := os.Stat(worktreesDir); err != nil {
			t.Errorf("expected legacy worktrees directory to exist: %v", err)
		}
		if filepath.Dir(repoDir) != rootDir {
			t.Errorf("repoDir parent mismatch: got %q, want %q", filepath.Dir(repoDir), rootDir)
		}
	})

	t.Run("CreateFeatureWorktreeInWorkspace", func(t *testing.T) {
		_, bareDir, _ := testutil.SetupPureBareWorkspace(t)
		wtDir := testutil.CreateFeatureWorktreeInWorkspace(t, bareDir, "feat-200-test", "feat/200-test")

		if _, err := os.Stat(wtDir); err != nil {
			t.Errorf("expected feature worktree to exist at %s: %v", wtDir, err)
		}

		branch := testutil.RunGit(t, wtDir, "branch", "--show-current")
		if branch != "feat/200-test" {
			t.Errorf("branch = %q, want %q", branch, "feat/200-test")
		}
	})
}
