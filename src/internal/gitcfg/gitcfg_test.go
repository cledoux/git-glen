package gitcfg_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git-glen/internal/gitcfg"
	"git-glen/internal/testutil"
)

func TestDiscoverAndLoad_ThreeStateDetection(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)

	t.Run("ImplicitBareWorkspace", func(t *testing.T) {
		rootDir, bareDir, mainDir := testutil.SetupPureBareWorkspace(t)
		wtDir := testutil.CreateFeatureWorktreeInWorkspace(t, bareDir, "feat-auth", "feat/auth")

		repoRoot, ok := gitcfg.Discover(rootDir)
		if !ok {
			t.Fatalf("Discover(rootDir) = false")
		}
		if !repoRoot.IsBareRoot || repoRoot.BaseDir != rootDir || repoRoot.CommonDir != bareDir {
			t.Errorf("unexpected repoRoot: %+v", repoRoot)
		}
		cfg := gitcfg.Load(repoRoot)
		if cfg.State != gitcfg.StateImplicitBare || !cfg.Initialized || cfg.Layout != gitcfg.LayoutBare {
			t.Errorf("unexpected implicit bare config: %+v", cfg)
		}
		if !cfg.ProtectMain || !cfg.AutoIsolate || cfg.DefaultBranch != "main" {
			t.Errorf("unexpected implicit bare flags: %+v", cfg)
		}

		repoMain, ok := gitcfg.Discover(filepath.Join(mainDir, "README.md"))
		if !ok {
			t.Fatalf("Discover(main/README.md) = false")
		}
		if !repoMain.IsLinkedWorktree || repoMain.CheckoutDir != mainDir || repoMain.CommonDir != bareDir || repoMain.BaseDir != rootDir {
			t.Errorf("unexpected repoMain: %+v", repoMain)
		}

		repoWT, ok := gitcfg.Discover(filepath.Join(wtDir, "nonexistent", "new.go"))
		if !ok {
			t.Fatalf("Discover(nonexistent leaf in worktree) = false")
		}
		if !repoWT.IsLinkedWorktree || repoWT.CheckoutDir != wtDir || repoWT.CommonDir != bareDir {
			t.Errorf("unexpected repoWT: %+v", repoWT)
		}
	})

	t.Run("UninitializedStandardClone", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)

		repo, ok := gitcfg.Discover(filepath.Join(cloneDir, "src", "app.go"))
		if !ok {
			t.Fatalf("Discover(cloneDir) = false")
		}
		if repo.IsBareRoot || repo.IsLinkedWorktree || repo.BaseDir != cloneDir || repo.CheckoutDir != cloneDir {
			t.Errorf("unexpected standard clone repo: %+v", repo)
		}

		cfg := gitcfg.Load(repo)
		if cfg.State != gitcfg.StateUninitialized || cfg.Initialized || cfg.Layout != "" {
			t.Errorf("expected uninitialized state, got %+v", cfg)
		}
		if cfg.ProtectMain || cfg.AutoIsolate {
			t.Errorf("expected ProtectMain=false and AutoIsolate=false on uninitialized clone, got %+v", cfg)
		}
	})

	t.Run("ConfiguredSubdirAndNestedLinkedWorktree", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		testutil.RunGit(t, cloneDir, "config", "glen.layout", "subdir")
		testutil.RunGit(t, cloneDir, "config", "glen.worktreesDir", ".worktrees")

		wtDir := filepath.Join(cloneDir, ".worktrees", "feat", "nested")
		if err := os.MkdirAll(filepath.Dir(wtDir), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		testutil.RunGit(t, cloneDir, "worktree", "add", wtDir, "-b", "feat/nested", "main")

		// Root checkout hits .git directory
		rootRepo, ok := gitcfg.Discover(filepath.Join(cloneDir, "README.md"))
		if !ok {
			t.Fatalf("Discover(root README) = false")
		}
		if rootRepo.IsLinkedWorktree || rootRepo.CheckoutDir != cloneDir || rootRepo.CommonDir != filepath.Join(cloneDir, ".git") {
			t.Errorf("unexpected rootRepo: %+v", rootRepo)
		}
		cfg := gitcfg.Load(rootRepo)
		if cfg.State != gitcfg.StateConfigured || !cfg.Initialized || cfg.Layout != gitcfg.LayoutSubdir {
			t.Errorf("unexpected configured subdir cfg: %+v", cfg)
		}
		if !cfg.ProtectMain || !cfg.AutoIsolate {
			t.Errorf("expected ProtectMain=true and AutoIsolate=true on initialized subdir, got %+v", cfg)
		}

		// Nested .worktrees/feat/nested hits its own .git pointer file first
		wtRepo, ok := gitcfg.Discover(filepath.Join(wtDir, "pkg", "new_file.go"))
		if !ok {
			t.Fatalf("Discover(nested worktree file) = false")
		}
		if !wtRepo.IsLinkedWorktree || wtRepo.CheckoutDir != wtDir || wtRepo.BaseDir != cloneDir || wtRepo.CommonDir != filepath.Join(cloneDir, ".git") {
			t.Errorf("unexpected wtRepo: %+v", wtRepo)
		}
		if b, ok := gitcfg.ReadHeadBranch(wtRepo.GitDir); !ok || b != "feat/nested" {
			t.Errorf("ReadHeadBranch(wtRepo.GitDir) = (%q, %v), want ('feat/nested', true)", b, ok)
		}
	})
}

func TestLoad_GlobalAndRepoPrecedence(t *testing.T) {
	globalCfg := filepath.Join(t.TempDir(), "gitconfig")
	content := `
# Global glen preferences
[glen]
	defaultLayout = subdir ; inline comment
	autoIsolate = true
	readOnlyAgents = "alpha, beta" gamma
`
	if err := os.WriteFile(globalCfg, []byte(content), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", globalCfg)

	cloneDir := testutil.SetupStandardClone(t, false)
	repo, ok := gitcfg.Discover(cloneDir)
	if !ok {
		t.Fatalf("Discover failed")
	}

	// Uninitialized repo inherits global autoIsolate=true and defaultLayout=subdir,
	// while remaining StateUninitialized with ProtectMain=false.
	cfg := gitcfg.Load(repo)
	if cfg.State != gitcfg.StateUninitialized || cfg.Initialized {
		t.Errorf("global config must not mark uninitialized repo as initialized: %+v", cfg)
	}
	if cfg.DefaultLayout != "subdir" || !cfg.AutoIsolate || cfg.ProtectMain {
		t.Errorf("unexpected global inheritance on uninitialized clone: %+v", cfg)
	}
	if len(cfg.ReadOnlyAgents) != 3 {
		t.Errorf("ReadOnlyAgents = %v, want 3 items", cfg.ReadOnlyAgents)
	}

	// Repo-level override wins over global
	testutil.RunGit(t, cloneDir, "config", "glen.autoIsolate", "false")
	cfgOverride := gitcfg.Load(repo)
	if cfgOverride.AutoIsolate {
		t.Errorf("expected repo glen.autoIsolate=false to override global true")
	}
}

func TestEnsureInfoExclude_Idempotent(t *testing.T) {
	commonDir := filepath.Join(t.TempDir(), ".git")
	if err := gitcfg.EnsureInfoExclude(commonDir, "/.worktrees/"); err != nil {
		t.Fatalf("first EnsureInfoExclude: %v", err)
	}
	if err := gitcfg.EnsureInfoExclude(commonDir, "/.worktrees/"); err != nil {
		t.Fatalf("second EnsureInfoExclude: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(commonDir, "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}
	if count := strings.Count(string(data), "/.worktrees/"); count != 1 {
		t.Errorf("exclude entry count = %d, want 1; content:\n%s", count, string(data))
	}
}
