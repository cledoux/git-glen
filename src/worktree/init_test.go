package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git-glen/internal/testutil"
)

func TestCheckWorkingTreeClean(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Run("CleanStandardClone", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		clean, dirtyFiles, err := CheckWorkingTreeClean(ctx, cloneDir)
		if err != nil {
			t.Fatalf("CheckWorkingTreeClean failed: %v", err)
		}
		if !clean {
			t.Errorf("expected clean working tree, got dirty with files: %v", dirtyFiles)
		}
		if len(dirtyFiles) != 0 {
			t.Errorf("expected 0 dirty files, got: %v", dirtyFiles)
		}
	})

	t.Run("DirtyStandardClone", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, true)
		clean, dirtyFiles, err := CheckWorkingTreeClean(ctx, cloneDir)
		if err != nil {
			t.Fatalf("CheckWorkingTreeClean failed: %v", err)
		}
		if clean {
			t.Errorf("expected dirty working tree, got clean")
		}
		if len(dirtyFiles) < 2 {
			t.Errorf("expected at least 2 dirty files (README.md and scratch.txt), got: %v", dirtyFiles)
		}
	})

	t.Run("CleanPureBareMain", func(t *testing.T) {
		rootDir, _, mainDir := testutil.SetupPureBareWorkspace(t)
		_ = rootDir
		clean, dirtyFiles, err := CheckWorkingTreeClean(ctx, mainDir)
		if err != nil {
			t.Fatalf("CheckWorkingTreeClean failed: %v", err)
		}
		if !clean {
			t.Errorf("expected clean main worktree, got dirty with files: %v", dirtyFiles)
		}
	})
}

func TestDetectScenario(t *testing.T) {
	t.Run("RemoteURL_GitSSH", func(t *testing.T) {
		scen, path, err := DetectScenario("git@github.com:example-org/git-glen.git")
		if err != nil {
			t.Fatalf("DetectScenario failed: %v", err)
		}
		if scen != ScenarioRemoteClone {
			t.Errorf("expected ScenarioRemoteClone, got %s", scen)
		}
		if path != "git-glen" {
			t.Errorf("expected target path 'git-glen', got '%s'", path)
		}
	})

	t.Run("RemoteURL_HTTPS", func(t *testing.T) {
		scen, path, err := DetectScenario("https://github.com/org/custom-repo.git")
		if err != nil {
			t.Fatalf("DetectScenario failed: %v", err)
		}
		if scen != ScenarioRemoteClone {
			t.Errorf("expected ScenarioRemoteClone, got %s", scen)
		}
		if path != "custom-repo" {
			t.Errorf("expected target path 'custom-repo', got '%s'", path)
		}
	})

	t.Run("PureBareWorkspace", func(t *testing.T) {
		rootDir, _, _ := testutil.SetupPureBareWorkspace(t)
		scen, path, err := DetectScenario(rootDir)
		if err != nil {
			t.Fatalf("DetectScenario failed: %v", err)
		}
		if scen != ScenarioBareRepair {
			t.Errorf("expected ScenarioBareRepair, got %s", scen)
		}
		if path != rootDir {
			t.Errorf("expected path '%s', got '%s'", rootDir, path)
		}
	})

	t.Run("StandardClone", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		scen, path, err := DetectScenario(cloneDir)
		if err != nil {
			t.Fatalf("DetectScenario failed: %v", err)
		}
		if scen != ScenarioCloneConvert {
			t.Errorf("expected ScenarioCloneConvert, got %s", scen)
		}
		if path != cloneDir {
			t.Errorf("expected path '%s', got '%s'", cloneDir, path)
		}
	})

	t.Run("LegacySiblingLayout", func(t *testing.T) {
		rootDir, _, _ := testutil.SetupLegacySiblingLayout(t)
		scen, path, err := DetectScenario(rootDir)
		if err != nil {
			t.Fatalf("DetectScenario failed: %v", err)
		}
		if scen != ScenarioLegacyMigrate {
			t.Errorf("expected ScenarioLegacyMigrate, got %s", scen)
		}
		if path != rootDir {
			t.Errorf("expected path '%s', got '%s'", rootDir, path)
		}
	})

	t.Run("GreenfieldEmptyDir", func(t *testing.T) {
		emptyDir := testutil.SetupGreenfieldDirectory(t)
		scen, path, err := DetectScenario(emptyDir)
		if err != nil {
			t.Fatalf("DetectScenario failed: %v", err)
		}
		if scen != ScenarioGreenfield {
			t.Errorf("expected ScenarioGreenfield, got %s", scen)
		}
		if path != emptyDir {
			t.Errorf("expected path '%s', got '%s'", emptyDir, path)
		}
	})

	t.Run("InvalidNonGitDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tmpDir, "somefile.txt"), []byte("data"), 0644)
		_, _, err := DetectScenario(tmpDir)
		if err == nil {
			t.Errorf("expected error for non-git non-empty dir, got nil")
		}
	})
}

func assertBareWorkspaceConfigAndPointer(t *testing.T, rootDir, bareDir string) {
	t.Helper()

	gitFile := filepath.Join(rootDir, ".git")
	info, err := os.Stat(gitFile)
	if err != nil {
		t.Fatalf("expected root .git pointer file at %s: %v", gitFile, err)
	}
	if info.IsDir() {
		t.Fatalf("expected root .git at %s to be a pointer file, got directory", gitFile)
	}
	data, err := os.ReadFile(gitFile)
	if err != nil {
		t.Fatalf("failed to read root .git pointer file: %v", err)
	}
	if strings.TrimSpace(string(data)) != "gitdir: ./.bare" {
		t.Errorf("expected root .git to contain 'gitdir: ./.bare', got %q", string(data))
	}

	expectedConfigs := map[string]string{
		"extensions.worktreeConfig": "true",
		"remote.origin.fetch":       "+refs/heads/*:refs/remotes/origin/*",
		"push.autoSetupRemote":      "true",
		"branch.autoSetupMerge":     "simple",
		"fetch.prune":               "true",
		"worktree.guessRemote":      "true",
	}
	for k, want := range expectedConfigs {
		got := testutil.RunGit(t, bareDir, "config", "--get", k)
		if got != want {
			t.Errorf("config %s = %q, want %q", k, got, want)
		}
	}
}

func TestInitBareRepo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Create a remote seed repo
	seedDir := t.TempDir()
	testutil.RunGit(t, seedDir, "init", "-b", "main")
	testutil.RunGit(t, seedDir, "config", "user.name", "Seed Author")
	testutil.RunGit(t, seedDir, "config", "user.email", "seed@example.com")
	_ = os.WriteFile(filepath.Join(seedDir, "README.md"), []byte("# Remote Seed\n"), 0644)
	testutil.RunGit(t, seedDir, "add", "README.md")
	testutil.RunGit(t, seedDir, "commit", "-m", "Seed commit on main")

	// 2. Init bare repo into targetDir
	targetParent := t.TempDir()
	targetDir := filepath.Join(targetParent, "cloned-project")

	mgr := NewManager("")
	res, err := mgr.InitBareRepo(ctx, seedDir, targetDir)
	if err != nil {
		t.Fatalf("InitBareRepo failed: %v", err)
	}

	if res.Scenario != ScenarioRemoteClone {
		t.Errorf("expected ScenarioRemoteClone, got %s", res.Scenario)
	}
	if res.BaseDir != targetDir {
		t.Errorf("expected BaseDir %s, got %s", targetDir, res.BaseDir)
	}
	if res.BareDir != filepath.Join(targetDir, ".bare") {
		t.Errorf("expected BareDir %s, got %s", filepath.Join(targetDir, ".bare"), res.BareDir)
	}
	if res.MainDir != filepath.Join(targetDir, "main") {
		t.Errorf("expected MainDir %s, got %s", filepath.Join(targetDir, "main"), res.MainDir)
	}
	if res.DefaultBranch != "main" {
		t.Errorf("expected DefaultBranch 'main', got '%s'", res.DefaultBranch)
	}

	// Verify .bare properties and root .git pointer
	bareDir := res.BareDir
	if !IsBareRepo(bareDir) {
		t.Errorf("expected .bare to be a bare repo")
	}
	assertBareWorkspaceConfigAndPointer(t, targetDir, bareDir)

	// Verify main worktree
	mainReadme := filepath.Join(targetDir, "main", "README.md")
	content, err := os.ReadFile(mainReadme)
	if err != nil || !strings.Contains(string(content), "Remote Seed") {
		t.Errorf("failed to read main/README.md or content mismatch: %v (content: %s)", err, string(content))
	}

	rootGemini := filepath.Join(targetDir, "GEMINI.md")
	geminiContent, err := os.ReadFile(rootGemini)
	if err != nil || !strings.Contains(string(geminiContent), PureBareGuardMarker) {
		t.Errorf("failed to read root GEMINI.md or missing guard marker: %v", err)
	}

	// Verify main/ worktree does NOT contain uncommitted GEMINI.md
	mainGemini := filepath.Join(targetDir, "main", "GEMINI.md")
	if _, err := os.Stat(mainGemini); !os.IsNotExist(err) {
		t.Errorf("expected no GEMINI.md dumped inside main/ worktree, found one")
	}

	// Verify no dedicated worktrees/ container is created
	wtDir := filepath.Join(targetDir, "worktrees")
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("expected no worktrees/ subdirectory in Pure Bare layout, got err=%v", err)
	}
}

func TestConvertStandardClone_Clean(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cloneDir := testutil.SetupStandardClone(t, false)

	// Add an extra tracked source file
	_ = os.WriteFile(filepath.Join(cloneDir, "main.go"), []byte("package main\n"), 0644)
	testutil.RunGit(t, cloneDir, "add", "main.go")
	testutil.RunGit(t, cloneDir, "commit", "-m", "Add main.go")

	mgr := NewManager("")
	res, err := mgr.ConvertStandardClone(ctx, cloneDir)
	if err != nil {
		t.Fatalf("ConvertStandardClone failed: %v", err)
	}

	if res.Scenario != ScenarioCloneConvert {
		t.Errorf("expected ScenarioCloneConvert, got %s", res.Scenario)
	}
	if !res.Converted {
		t.Errorf("expected res.Converted to be true")
	}

	// Verify .bare exists and root .git is now a pointer file to ./.bare
	bareDir := filepath.Join(cloneDir, ".bare")
	if !IsBareRepo(bareDir) {
		t.Errorf("expected %s to be bare repo", bareDir)
	}
	assertBareWorkspaceConfigAndPointer(t, cloneDir, bareDir)

	// Verify main/ contains tracked files
	mainDir := filepath.Join(cloneDir, "main")
	if _, err := os.Stat(filepath.Join(mainDir, "main.go")); err != nil {
		t.Errorf("expected main/main.go to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mainDir, "README.md")); err != nil {
		t.Errorf("expected main/README.md to exist: %v", err)
	}

	// Verify git log in main
	logOut := testutil.RunGit(t, mainDir, "log", "-1", "--format=%s")
	if logOut != "Add main.go" {
		t.Errorf("expected latest commit 'Add main.go', got '%s'", logOut)
	}

	// Verify root GEMINI.md exists
	rootGemini := filepath.Join(cloneDir, "GEMINI.md")
	if _, err := os.Stat(rootGemini); err != nil {
		t.Errorf("expected root GEMINI.md to exist: %v", err)
	}
	// Verify main/ worktree does NOT contain GEMINI.md
	if _, err := os.Stat(filepath.Join(mainDir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Errorf("expected main/ worktree to NOT contain GEMINI.md")
	}

	// Verify no dedicated worktrees/ container is created
	wtDir := filepath.Join(cloneDir, "worktrees")
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("expected no worktrees/ subdirectory in Pure Bare layout, got err=%v", err)
	}
}

func TestConvertStandardClone_Dirty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cloneDir := testutil.SetupStandardClone(t, true)

	mgr := NewManager("")
	_, err := mgr.ConvertStandardClone(ctx, cloneDir)
	if err == nil {
		t.Fatalf("expected error on dirty clone, got nil")
	}

	var dirtyErr *DirtyTreeError
	if !errors.As(err, &dirtyErr) {
		t.Fatalf("expected *DirtyTreeError, got: %T (%v)", err, err)
	}

	if len(dirtyErr.Files) < 2 {
		t.Errorf("expected at least 2 dirty files in error, got: %v", dirtyErr.Files)
	}

	// Verify .git was NOT modified or moved
	if _, err := os.Stat(filepath.Join(cloneDir, ".git")); os.IsNotExist(err) {
		t.Errorf("expected .git directory to still exist after rejected conversion")
	}
	if _, err := os.Stat(filepath.Join(cloneDir, ".bare")); !os.IsNotExist(err) {
		t.Errorf("expected .bare to not be created on rejection")
	}
}

func TestRepairWorkspace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rootDir, bareDir, mainDir := testutil.SetupPureBareWorkspace(t)
	_ = mainDir

	// Simulate broken config by removing refspec and root .git pointer
	testutil.RunGit(t, bareDir, "config", "--unset", "remote.origin.fetch")
	_ = os.Remove(filepath.Join(rootDir, ".git"))

	mgr := NewManager(bareDir)
	res, err := mgr.RepairWorkspace(ctx, rootDir)
	if err != nil {
		t.Fatalf("RepairWorkspace failed: %v", err)
	}

	if res.Scenario != ScenarioBareRepair {
		t.Errorf("expected ScenarioBareRepair, got %s", res.Scenario)
	}
	if !res.Repaired {
		t.Errorf("expected res.Repaired to be true")
	}

	// Verify restored refspec, default configs, and root .git pointer
	assertBareWorkspaceConfigAndPointer(t, rootDir, bareDir)

	// Verify idempotent subsequent execution
	res2, err := mgr.RepairWorkspace(ctx, rootDir)
	if err != nil {
		t.Fatalf("subsequent RepairWorkspace failed: %v", err)
	}
	if !res2.Repaired {
		t.Errorf("expected res2.Repaired true")
	}
}

func TestMigrateLegacySibling_Clean(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rootDir, repoDir, worktreesDir := testutil.SetupLegacySiblingLayout(t)
	_ = repoDir
	_ = worktreesDir

	mgr := NewManager("")
	res, err := mgr.MigrateLegacySibling(ctx, rootDir)
	if err != nil {
		t.Fatalf("MigrateLegacySibling failed: %v", err)
	}

	if res.Scenario != ScenarioLegacyMigrate {
		t.Errorf("expected ScenarioLegacyMigrate, got %s", res.Scenario)
	}
	if !res.Migrated {
		t.Errorf("expected res.Migrated to be true")
	}

	// Verify repo/ is gone and main/ exists
	if _, err := os.Stat(filepath.Join(rootDir, "repo")); !os.IsNotExist(err) {
		t.Errorf("expected legacy repo/ to be removed")
	}
	if _, err := os.Stat(filepath.Join(rootDir, "main", "README.md")); err != nil {
		t.Errorf("expected main/README.md to exist: %v", err)
	}
	bareDir := filepath.Join(rootDir, ".bare")
	if !IsBareRepo(bareDir) {
		t.Errorf("expected .bare to be bare repo")
	}
	assertBareWorkspaceConfigAndPointer(t, rootDir, bareDir)
}

func TestMigrateLegacySibling_Dirty(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rootDir, repoDir, _ := testutil.SetupLegacySiblingLayout(t)
	_ = os.WriteFile(filepath.Join(repoDir, "scratch.txt"), []byte("dirty\n"), 0644)

	mgr := NewManager("")
	_, err := mgr.MigrateLegacySibling(ctx, rootDir)
	if err == nil {
		t.Fatalf("expected error on dirty legacy repo, got nil")
	}

	var dirtyErr *DirtyTreeError
	if !errors.As(err, &dirtyErr) {
		t.Fatalf("expected *DirtyTreeError, got %T (%v)", err, err)
	}
}

func TestInitGreenfield(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	emptyDir := testutil.SetupGreenfieldDirectory(t)

	mgr := NewManager("")
	res, err := mgr.InitGreenfield(ctx, emptyDir)
	if err != nil {
		t.Fatalf("InitGreenfield failed: %v", err)
	}

	if res.Scenario != ScenarioGreenfield {
		t.Errorf("expected ScenarioGreenfield, got %s", res.Scenario)
	}

	bareDir := filepath.Join(emptyDir, ".bare")
	if !IsBareRepo(bareDir) {
		t.Errorf("expected %s to be bare repo", bareDir)
	}
	assertBareWorkspaceConfigAndPointer(t, emptyDir, bareDir)

	mainDir := filepath.Join(emptyDir, "main")
	if _, err := os.Stat(mainDir); err != nil {
		t.Errorf("expected main/ worktree to exist: %v", err)
	}

	wtDir := filepath.Join(emptyDir, "worktrees")
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Errorf("expected no worktrees/ subdirectory in Pure Bare layout, got err=%v", err)
	}

	// Verify root GEMINI.md exists
	rootGemini := filepath.Join(emptyDir, "GEMINI.md")
	if _, err := os.Stat(rootGemini); err != nil {
		t.Errorf("expected root GEMINI.md to exist: %v", err)
	}
	// Verify main/ worktree does NOT contain GEMINI.md
	if _, err := os.Stat(filepath.Join(mainDir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Errorf("expected main/ worktree to NOT contain GEMINI.md")
	}
	// Verify main/ initial commit does NOT include GEMINI.md
	logFiles := testutil.RunGit(t, mainDir, "log", "-1", "--name-only", "--pretty=format:")
	if strings.Contains(logFiles, "GEMINI.md") {
		t.Errorf("expected initial commit to NOT include GEMINI.md, got files:\n%s", logFiles)
	}
}

func TestDirtyTreeErrorFormatting(t *testing.T) {
	err := &DirtyTreeError{
		Dir:   "/path/to/repo",
		Files: []string{"?? scratch.txt", "M  main.go"},
	}

	msg := err.Error()
	if !strings.Contains(msg, "/path/to/repo") {
		t.Errorf("expected dir in error message, got: %s", msg)
	}
	if !strings.Contains(msg, "scratch.txt") || !strings.Contains(msg, "main.go") {
		t.Errorf("expected dirty files in error message, got: %s", msg)
	}
}

func TestInitDispatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mgr := NewManager("")

	// Dispatch to greenfield
	emptyDir := testutil.SetupGreenfieldDirectory(t)
	res, err := mgr.Init(ctx, emptyDir)
	if err != nil {
		t.Fatalf("mgr.Init on greenfield failed: %v", err)
	}
	if res.Scenario != ScenarioGreenfield {
		t.Errorf("expected ScenarioGreenfield, got %s", res.Scenario)
	}

	// Dispatch to bare repair on the same directory
	res2, err := mgr.Init(ctx, emptyDir)
	if err != nil {
		t.Fatalf("mgr.Init on repaired bare failed: %v", err)
	}
	if res2.Scenario != ScenarioBareRepair {
		t.Errorf("expected ScenarioBareRepair, got %s", res2.Scenario)
	}
}

func TestEnsureWorkspaceGuard(t *testing.T) {
	t.Run("CreatesGEMINIWhenMissing", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureWorkspaceGuard(dir); err != nil {
			t.Fatalf("EnsureWorkspaceGuard failed: %v", err)
		}
		content, err := os.ReadFile(filepath.Join(dir, "GEMINI.md"))
		if err != nil {
			t.Fatalf("failed to read created GEMINI.md: %v", err)
		}
		if !strings.Contains(string(content), PureBareGuardMarker) {
			t.Errorf("expected GEMINI.md to contain marker %s", PureBareGuardMarker)
		}
		if !strings.Contains(string(content), "Pure Bare Worktree Discipline") {
			t.Errorf("expected GEMINI.md to contain tripwire heading")
		}
	})

	t.Run("AppendsToExistingGEMINI", func(t *testing.T) {
		dir := t.TempDir()
		geminiPath := filepath.Join(dir, "GEMINI.md")
		initial := "# Custom Project\n\nExisting instructions.\n"
		if err := os.WriteFile(geminiPath, []byte(initial), 0644); err != nil {
			t.Fatalf("failed to write initial GEMINI.md: %v", err)
		}

		if err := EnsureWorkspaceGuard(dir); err != nil {
			t.Fatalf("EnsureWorkspaceGuard failed: %v", err)
		}

		content, err := os.ReadFile(geminiPath)
		if err != nil {
			t.Fatalf("failed to read GEMINI.md: %v", err)
		}
		str := string(content)
		if !strings.HasPrefix(str, "# Custom Project") {
			t.Errorf("expected initial content preserved, got: %s", str)
		}
		if !strings.Contains(str, PureBareGuardMarker) {
			t.Errorf("expected marker in content")
		}

		// Idempotency: call again, ensure not duplicated
		if err := EnsureWorkspaceGuard(dir); err != nil {
			t.Fatalf("second EnsureWorkspaceGuard failed: %v", err)
		}
		content2, err := os.ReadFile(geminiPath)
		if err != nil {
			t.Fatalf("failed to read GEMINI.md on second call: %v", err)
		}
		if strings.Count(string(content2), PureBareGuardMarker) != 1 {
			t.Errorf("expected exactly 1 marker after repeated call, got %d", strings.Count(string(content2), PureBareGuardMarker))
		}
	})

	t.Run("AppendsToExistingAGENTSWhenPresent", func(t *testing.T) {
		dir := t.TempDir()
		agentsPath := filepath.Join(dir, "AGENTS.md")
		initial := "# Agent Guidelines\n"
		if err := os.WriteFile(agentsPath, []byte(initial), 0644); err != nil {
			t.Fatalf("failed to write initial AGENTS.md: %v", err)
		}

		if err := EnsureWorkspaceGuard(dir); err != nil {
			t.Fatalf("EnsureWorkspaceGuard failed: %v", err)
		}

		content, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("failed to read AGENTS.md: %v", err)
		}
		if !strings.Contains(string(content), PureBareGuardMarker) {
			t.Errorf("expected marker in AGENTS.md")
		}
		// Verify GEMINI.md was not redundantly created
		if _, err := os.Stat(filepath.Join(dir, "GEMINI.md")); !os.IsNotExist(err) {
			t.Errorf("expected GEMINI.md to not be created when AGENTS.md exists")
		}
	})
}
