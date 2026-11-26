package worktree_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git-glen/internal/testutil"
	"git-glen/worktree"
)

func TestDiscoverTopology(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)

	t.Run("PureBareRoot", func(t *testing.T) {
		rootDir, bareDir, mainDir := testutil.SetupPureBareWorkspace(t)

		topo, err := worktree.DiscoverTopology(rootDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from root failed: %v", err)
		}
		if !topo.IsBare || !topo.Initialized || topo.Layout != worktree.LayoutBare || topo.BareDir != bareDir || topo.WorktreesDir != rootDir {
			t.Errorf("unexpected topology at root: %+v", topo)
		}

		topoMain, err := worktree.DiscoverTopology(mainDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from main failed: %v", err)
		}
		if !topoMain.IsWorktree || topoMain.CurrentWorktree != "main" || topoMain.Layout != worktree.LayoutBare {
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

	t.Run("StandardClone_Uninitialized", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		topo, err := worktree.DiscoverTopology(cloneDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology failed: %v", err)
		}
		wantWTDir := filepath.Join(cloneDir, worktree.DefaultSubdirWorktreesDir)
		if topo.IsBare || topo.Initialized || topo.Layout != "" || topo.RepoDir != filepath.Join(cloneDir, ".git") || topo.WorktreesDir != wantWTDir {
			t.Errorf("unexpected uninitialized standard clone topology: %+v", topo)
		}
	})

	t.Run("SubdirLayout_RootAndLinkedWorktree", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		cloneDir := testutil.SetupStandardClone(t, false)
		mgr := worktree.NewManager("")
		res, err := mgr.InitWithOptions(ctx, cloneDir, worktree.InitOptions{Layout: worktree.LayoutSubdir})
		if err != nil {
			t.Fatalf("InitWithOptions(subdir) failed: %v", err)
		}

		topoRoot, err := worktree.DiscoverTopology(cloneDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from subdir root failed: %v", err)
		}
		if !topoRoot.Initialized || topoRoot.Layout != worktree.LayoutSubdir || topoRoot.IsBare || topoRoot.IsWorktree {
			t.Errorf("unexpected subdir root topology: %+v", topoRoot)
		}
		if topoRoot.WorktreesDir != res.WorktreesDir {
			t.Errorf("WorktreesDir = %q, want %q", topoRoot.WorktreesDir, res.WorktreesDir)
		}

		wtMgr := worktree.NewManager(topoRoot.RepoDir)
		wtPath, _, err := wtMgr.CreateFeatureWorktree(ctx, 101, "auth", "main", "", "")
		if err != nil {
			t.Fatalf("CreateFeatureWorktree in subdir failed: %v", err)
		}
		subDir := filepath.Join(wtPath, "internal", "pkg")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("mkdir subdir: %v", err)
		}

		topoWT, err := worktree.DiscoverTopology(subDir, "")
		if err != nil {
			t.Fatalf("DiscoverTopology from nested subdir worktree failed: %v", err)
		}
		if !topoWT.Initialized || topoWT.Layout != worktree.LayoutSubdir || !topoWT.IsWorktree || topoWT.BaseDir != cloneDir || topoWT.CurrentWorktree != "feat/101-auth" {
			t.Errorf("unexpected subdir linked worktree topology: %+v", topoWT)
		}
	})

	t.Run("InvalidConfiguredLayout", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		testutil.RunGit(t, cloneDir, "config", "glen.layout", "nonexistent")
		if _, err := worktree.DiscoverTopology(cloneDir, ""); err == nil {
			t.Errorf("expected error for unsupported glen.layout")
		}
	})
}
