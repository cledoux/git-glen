package worktree_test

import (
	"path/filepath"
	"testing"

	"git-glen/internal/testutil"
	"git-glen/worktree"
)

func TestDiscoverTopology(t *testing.T) {
	t.Run("PureBareRoot", func(t *testing.T) {
		rootDir, bareDir, mainDir := testutil.SetupPureBareWorkspace(t)

		topo, err := worktree.DiscoverTopology(rootDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from root failed: %v", err)
		}
		if !topo.IsBare || topo.BareDir != bareDir || topo.WorktreesDir != rootDir {
			t.Errorf("unexpected topology at root: %+v", topo)
		}

		topoMain, err := worktree.DiscoverTopology(mainDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from main failed: %v", err)
		}
		if !topoMain.IsWorktree || topoMain.CurrentWorktree != "main" {
			t.Errorf("expected CurrentWorktree='main', got %+v", topoMain)
		}

		wtDir := testutil.CreateFeatureWorktreeInWorkspace(t, bareDir, "feat-101-auth", "feat/101-auth")
		topoWT, err := worktree.DiscoverTopology(wtDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from nested branch worktree failed: %v", err)
		}
		if !topoWT.IsWorktree || topoWT.CurrentWorktree != "feat/101-auth" {
			t.Errorf("expected CurrentWorktree='feat/101-auth', got %+v", topoWT)
		}
	})

	t.Run("StandardClone", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		topo, err := worktree.DiscoverTopology(cloneDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology failed: %v", err)
		}
		if topo.IsBare || topo.RepoDir != filepath.Join(cloneDir, ".git") {
			t.Errorf("unexpected standard clone topology: %+v", topo)
		}
	})
}
