package hook

import (
	"os"
	"path/filepath"
	"testing"
)

// setupWorkspace builds a minimal on-disk bare layout. The guard only
// inspects the filesystem, so no git subprocess is needed.
func setupWorkspace(t *testing.T, head string) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root := t.TempDir()
	for _, d := range []string{".bare", "main", "trunk", "feat/x"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if head != "" {
		if err := os.WriteFile(filepath.Join(root, ".bare", "HEAD"), []byte(head), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
	}
	return root
}

// setupSubdirWorkspace builds a minimal on-disk subdir layout (.git/ + .worktrees/feat/x).
func setupSubdirWorkspace(t *testing.T, head, extraConfig string) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	wtGitDir := filepath.Join(gitDir, "worktrees", "x")
	wtCheckout := filepath.Join(repo, ".worktrees", "feat", "x")

	for _, d := range []string{filepath.Join(repo, "src"), wtGitDir, wtCheckout} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	cfg := "[glen]\n\tlayout = subdir\n" + extraConfig
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if head != "" {
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatalf("write commondir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtGitDir, "HEAD"), []byte("ref: refs/heads/feat/x\n"), 0o644); err != nil {
		t.Fatalf("write worktree HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtCheckout, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0o644); err != nil {
		t.Fatalf("write worktree .git pointer: %v", err)
	}
	return repo
}

func TestEvaluateDefaultBranchEdits(t *testing.T) {
	root := setupWorkspace(t, "ref: refs/heads/main\n")
	plain := t.TempDir()

	tests := []struct {
		name     string
		action   Action
		wantDeny bool
	}{
		{"AbsoluteInMain", Action{Paths: []string{filepath.Join(root, "main", "src", "a.go")}}, true},
		{"MainDirItself", Action{Paths: []string{filepath.Join(root, "main")}}, true},
		{"RelativeFromMain", Action{Cwd: filepath.Join(root, "main"), Paths: []string{"README.md"}}, true},
		{"RelativeEscapesToMain", Action{Cwd: filepath.Join(root, "feat", "x"), Paths: []string{"../../main/a.go"}}, true},
		{"FeatureWorktree", Action{Paths: []string{filepath.Join(root, "feat", "x", "a.go")}}, false},
		{"SiblingWithMainPrefix", Action{Paths: []string{filepath.Join(root, "mainline", "a.go")}}, false},
		{"WorkspaceRootFile", Action{Paths: []string{filepath.Join(root, "CLAUDE.md")}}, false},
		{"OutsideWorkspace", Action{Paths: []string{filepath.Join(plain, "a.go")}}, false},
		{"RelativeWithoutCwd", Action{Paths: []string{"main/a.go"}}, false},
		{"NoPaths", Action{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.action)
			if got.Deny != tt.wantDeny {
				t.Errorf("Evaluate(%+v).Deny = %v, want %v (reason: %q)", tt.action, got.Deny, tt.wantDeny, got.Reason)
			}
			if got.Deny && got.Reason == "" {
				t.Errorf("denial must carry a reason")
			}
		})
	}
}

func TestEvaluateDefaultBranchEditsViaSymlink(t *testing.T) {
	root := setupWorkspace(t, "ref: refs/heads/main\n")

	if err := os.MkdirAll(filepath.Join(root, "main", ".agents", "rules"), 0o755); err != nil {
		t.Fatalf("mkdir main/.agents/rules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main", ".agents", "rules", "sdd.md"), []byte("# sdd\n"), 0o644); err != nil {
		t.Fatalf("write sdd.md: %v", err)
	}
	if err := os.Symlink(filepath.Join("main", ".agents"), filepath.Join(root, ".agents")); err != nil {
		t.Fatalf("symlink .agents: %v", err)
	}
	if err := os.Symlink(filepath.Join("main", "AGENTS.md"), filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("symlink broken AGENTS.md: %v", err)
	}
	if err := os.Symlink(filepath.Join("feat", "x"), filepath.Join(root, "feat-link")); err != nil {
		t.Fatalf("symlink feat-link: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantDeny bool
	}{
		{"ExistingFileViaRootAgentsSymlink", filepath.Join(root, ".agents", "rules", "sdd.md"), true},
		{"NewFileViaRootAgentsSymlink", filepath.Join(root, ".agents", "rules", "new.md"), true},
		{"BrokenSymlinkTargetingMain", filepath.Join(root, "AGENTS.md"), true},
		{"SymlinkTargetingFeatureWorktree", filepath.Join(root, "feat-link", "a.go"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(Action{Paths: []string{tt.path}})
			if got.Deny != tt.wantDeny {
				t.Errorf("Evaluate(%s).Deny = %v, want %v (reason: %q)", tt.path, got.Deny, tt.wantDeny, got.Reason)
			}
		})
	}
}

func TestEvaluateFollowsBareHEAD(t *testing.T) {
	root := setupWorkspace(t, "ref: refs/heads/trunk\n")

	if d := Evaluate(Action{Paths: []string{filepath.Join(root, "trunk", "a.go")}}); !d.Deny {
		t.Errorf("expected edit in trunk/ to be denied when HEAD is trunk")
	}
	if d := Evaluate(Action{Paths: []string{filepath.Join(root, "main", "a.go")}}); d.Deny {
		t.Errorf("expected edit in main/ to be allowed when HEAD is trunk, got %q", d.Reason)
	}
}

func TestEvaluateMissingHEADDefaultsToMain(t *testing.T) {
	root := setupWorkspace(t, "")
	if d := Evaluate(Action{Paths: []string{filepath.Join(root, "main", "a.go")}}); !d.Deny {
		t.Errorf("expected edit in main/ to be denied when HEAD is unreadable")
	}
}

func TestEvaluateOverrideEnv(t *testing.T) {
	root := setupWorkspace(t, "ref: refs/heads/main\n")
	t.Setenv(EnvAllowDefaultBranchEdits, "1")
	if d := Evaluate(Action{Paths: []string{filepath.Join(root, "main", "a.go")}}); d.Deny {
		t.Errorf("expected %s to allow edits in main/, got %q", EnvAllowDefaultBranchEdits, d.Reason)
	}
}

func TestEvaluateSubdirLayout_GuardsMainAndAllowsWorktrees(t *testing.T) {
	repo := setupSubdirWorkspace(t, "ref: refs/heads/main\n", "")
	wtDir := filepath.Join(repo, ".worktrees", "feat", "x")

	if err := os.Symlink(filepath.Join(repo, "src"), filepath.Join(wtDir, "escape")); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}

	tests := []struct {
		name     string
		action   Action
		wantDeny bool
	}{
		{"RootCheckoutFile", Action{Paths: []string{filepath.Join(repo, "src", "app.go")}}, true},
		{"RelativeFromRootCheckout", Action{Cwd: repo, Paths: []string{"README.md"}}, true},
		{"LinkedSubdirWorktreeExistingDir", Action{Paths: []string{filepath.Join(wtDir, "a.go")}}, false},
		{"LinkedSubdirWorktreeNewNestedDir", Action{Paths: []string{filepath.Join(wtDir, "pkg", "new", "b.go")}}, false},
		{"SymlinkEscapeFromWorktreeToRoot", Action{Paths: []string{filepath.Join(wtDir, "escape", "app.go")}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.action)
			if got.Deny != tt.wantDeny {
				t.Errorf("Evaluate(%+v).Deny = %v, want %v (reason: %q)", tt.action, got.Deny, tt.wantDeny, got.Reason)
			}
		})
	}
}

func TestEvaluateSubdirLayout_AllowsNonDefaultBranchAndProtectMainFalse(t *testing.T) {
	t.Run("RootCheckedOutOnFeatureBranch", func(t *testing.T) {
		repo := setupSubdirWorkspace(t, "ref: refs/heads/feat/local\n", "\tdefaultBranch = main\n")
		if d := Evaluate(Action{Paths: []string{filepath.Join(repo, "src", "app.go")}}); d.Deny {
			t.Errorf("expected edit on non-default branch in subdir root to be allowed, got %q", d.Reason)
		}
	})

	t.Run("ProtectMainDisabledInConfig", func(t *testing.T) {
		repo := setupSubdirWorkspace(t, "ref: refs/heads/main\n", "\tprotectMain = false\n")
		if d := Evaluate(Action{Paths: []string{filepath.Join(repo, "src", "app.go")}}); d.Deny {
			t.Errorf("expected protectMain=false to allow main edits, got %q", d.Reason)
		}
	})

	t.Run("UninitializedStandardClonePassesThrough", func(t *testing.T) {
		t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
		clone := t.TempDir()
		if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		if err := os.WriteFile(filepath.Join(clone, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
		if err := os.WriteFile(filepath.Join(clone, ".git", "config"), []byte("[core]\n\tbare = false\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
		if d := Evaluate(Action{Paths: []string{filepath.Join(clone, "README.md")}}); d.Deny {
			t.Errorf("expected uninitialized clone to allow edits on main, got %q", d.Reason)
		}
		if _, ok := FindWorkspace(clone); ok {
			t.Errorf("expected FindWorkspace=false on uninitialized clone")
		}
	})
}

func TestEvaluateDeleteBranchMerge(t *testing.T) {
	tests := []struct {
		command  string
		wantDeny bool
	}{
		{"gh pr merge 12 --squash --delete-branch", true},
		{"gh pr merge --squash -d", true},
		{"gh pr merge 12 --delete-branch=true", true},
		{"cd main && gh pr merge 12 -d", true},
		{"gh pr merge 12 --squash", false},
		{"gh pr view 12 && git branch -d feat/x", false},
		{"git branch -d feat/x", false},
		{"echo gh pr merge", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			got := Evaluate(Action{Command: tt.command})
			if got.Deny != tt.wantDeny {
				t.Errorf("Evaluate(%q).Deny = %v, want %v", tt.command, got.Deny, tt.wantDeny)
			}
		})
	}
}
