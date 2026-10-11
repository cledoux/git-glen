package hook

import (
	"os"
	"path/filepath"
	"testing"
)

// setupWorkspace builds a minimal on-disk Pure Bare layout. The guard only
// inspects the filesystem, so no git repository is needed.
func setupWorkspace(t *testing.T, head string) string {
	t.Helper()
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
