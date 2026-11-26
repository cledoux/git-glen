package worktree

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git-glen/internal/testutil"
)

func TestSubdir_DirtyStandardCloneInPlaceAndSubsequentRepair(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Dirty clone: modified tracked README.md + untracked scratch.txt
	cloneDir := testutil.SetupStandardClone(t, true)

	mgr := NewManager("")
	res, err := mgr.InitWithOptions(ctx, cloneDir, InitOptions{Layout: LayoutSubdir})
	if err != nil {
		t.Fatalf("InitWithOptions(subdir) on dirty clone failed: %v", err)
	}
	if res.Scenario != ScenarioSubdirInit || res.Layout != LayoutSubdir || res.Repaired {
		t.Errorf("unexpected InitResult: %+v", res)
	}
	wantWT := filepath.Join(cloneDir, DefaultSubdirWorktreesDir)
	if res.WorktreesDir != wantWT {
		t.Errorf("WorktreesDir = %q, want %q", res.WorktreesDir, wantWT)
	}

	// Verify dirty files remain untouched and .worktrees/ is excluded from git status
	scratchData, err := os.ReadFile(filepath.Join(cloneDir, "scratch.txt"))
	if err != nil || !strings.Contains(string(scratchData), "untracked content") {
		t.Errorf("expected scratch.txt to be preserved, got err=%v", err)
	}
	// Write a dummy file inside .worktrees/ to prove .git/info/exclude hides it from git status
	if err := os.WriteFile(filepath.Join(wantWT, "probe.txt"), []byte("ignored\n"), 0o644); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	statusOut := testutil.RunGit(t, cloneDir, "status", "--porcelain")
	if strings.Contains(statusOut, ".worktrees") || strings.Contains(statusOut, "probe.txt") {
		t.Errorf("expected .worktrees/ to be ignored via .git/info/exclude, got status:\n%s", statusOut)
	}
	_ = os.Remove(filepath.Join(wantWT, "probe.txt"))

	// Subsequent plain Init() WITHOUT Layout option MUST preserve subdir layout and run subdir-repair
	res2, err := mgr.Init(ctx, cloneDir)
	if err != nil {
		t.Fatalf("subsequent Init() on subdir repo failed: %v", err)
	}
	if res2.Scenario != ScenarioSubdirRepair || res2.Layout != LayoutSubdir || !res2.Repaired {
		t.Errorf("expected ScenarioSubdirRepair preserving subdir layout, got %+v", res2)
	}
}

func TestSubdir_RemoteCloneGreenfieldAndGlobalDefault(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("GreenfieldWithGlobalDefaultSubdir", func(t *testing.T) {
		globalCfg := filepath.Join(t.TempDir(), "gitconfig")
		if err := os.WriteFile(globalCfg, []byte("[glen]\n\tdefaultLayout = subdir\n"), 0o644); err != nil {
			t.Fatalf("write global config: %v", err)
		}
		t.Setenv("GIT_CONFIG_GLOBAL", globalCfg)

		emptyDir := testutil.SetupGreenfieldDirectory(t)
		mgr := NewManager("")
		res, err := mgr.Init(ctx, emptyDir)
		if err != nil {
			t.Fatalf("Init on greenfield with global subdir default failed: %v", err)
		}
		if res.Scenario != ScenarioGreenfield || res.Layout != LayoutSubdir {
			t.Errorf("unexpected result: %+v", res)
		}
		if got := testutil.RunGit(t, emptyDir, "config", "--get", "glen.layout"); got != "subdir" {
			t.Errorf("glen.layout = %q, want 'subdir'", got)
		}
	})

	t.Run("RemoteCloneSubdir", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
		seedDir := testutil.SetupStandardClone(t, false)
		targetDir := filepath.Join(t.TempDir(), "cloned-subdir")

		mgr := NewManager("")
		res, err := mgr.InitWithOptions(ctx, seedDir, InitOptions{Layout: LayoutSubdir})
		if err != nil {
			t.Fatalf("seed init failed: %v", err)
		}
		_ = res

		resClone, err := mgr.InitSubdirRepo(ctx, seedDir, targetDir, ".glen/worktrees")
		if err != nil {
			t.Fatalf("InitSubdirRepo failed: %v", err)
		}
		if resClone.Scenario != ScenarioRemoteClone || resClone.Layout != LayoutSubdir {
			t.Errorf("unexpected clone result: %+v", resClone)
		}
		wantWT := filepath.Join(targetDir, ".glen", "worktrees")
		if resClone.WorktreesDir != wantWT {
			t.Errorf("WorktreesDir = %q, want %q", resClone.WorktreesDir, wantWT)
		}
		excludeBytes, err := os.ReadFile(filepath.Join(targetDir, ".git", "info", "exclude"))
		if err != nil || !strings.Contains(string(excludeBytes), "/.glen/worktrees/") {
			t.Errorf("expected /.glen/worktrees/ in .git/info/exclude, got %q (err=%v)", string(excludeBytes), err)
		}
	})
}

func TestSubdir_WorktreeLifecycleVSCodeAndBareConvertGuard(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cloneDir := testutil.SetupStandardClone(t, false)
	initMgr := NewManager("")
	res, err := initMgr.InitWithOptions(ctx, cloneDir, InitOptions{
		Layout: LayoutSubdir,
		VSCode: true,
	})
	if err != nil {
		t.Fatalf("InitWithOptions(subdir, vscode) failed: %v", err)
	}
	if res.VSCodeWorkspace == "" {
		t.Fatalf("expected VSCodeWorkspace to be created")
	}

	mgr := NewManager(filepath.Join(cloneDir, ".git"))
	wt1, br1, err := mgr.CreateFeatureWorktree(ctx, 201, "auth", "main", "Archer", "user+archer@example.com")
	if err != nil {
		t.Fatalf("CreateFeatureWorktree 201 failed: %v", err)
	}
	wantWT1 := filepath.Join(cloneDir, ".worktrees", "feat", "201-auth")
	if wt1 != wantWT1 || br1 != "feat/201-auth" {
		t.Errorf("got (%q, %q), want (%q, 'feat/201-auth')", wt1, br1, wantWT1)
	}

	// Verify attempting to convert to bare while linked worktrees exist fails safely
	if _, err := initMgr.InitWithOptions(ctx, cloneDir, InitOptions{Layout: LayoutBare}); err == nil {
		t.Fatalf("expected ConvertStandardClone to refuse while linked worktree exists")
	}
	if _, err := os.Stat(wt1); err != nil {
		t.Fatalf("linked worktree %s was damaged by rejected conversion: %v", wt1, err)
	}

	// Verify VS Code workspace lists primary root "." and ".worktrees/feat/201-auth"
	wsData, err := os.ReadFile(res.VSCodeWorkspace)
	if err != nil {
		t.Fatalf("read .code-workspace: %v", err)
	}
	var parsed struct {
		Folders []VSCodeFolder `json:"folders"`
	}
	if err := json.Unmarshal(wsData, &parsed); err != nil {
		t.Fatalf("unmarshal .code-workspace: %v", err)
	}
	if len(parsed.Folders) != 2 || parsed.Folders[0].Path != "." || parsed.Folders[1].Path != ".worktrees/feat/201-auth" || parsed.Folders[1].Name != "feat/201-auth" {
		t.Errorf("unexpected VS Code folders: %+v", parsed.Folders)
	}

	// Remove worktree and verify empty intermediate feat/ dir is cleaned while .worktrees/ remains
	if err := mgr.RemoveFeatureWorktree(ctx, wt1, br1); err != nil {
		t.Fatalf("RemoveFeatureWorktree failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneDir, ".worktrees", "feat")); !os.IsNotExist(err) {
		t.Errorf("expected empty .worktrees/feat directory to be removed")
	}
	if _, err := os.Stat(filepath.Join(cloneDir, ".worktrees")); err != nil {
		t.Errorf("expected .worktrees root directory to remain: %v", err)
	}
}
