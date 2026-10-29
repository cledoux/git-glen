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

func TestRunGlenCommand_VSCodeWorkspaceSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	emptyDir := testutil.SetupGreenfieldDirectory(t)
	wsPath := filepath.Join(emptyDir, filepath.Base(emptyDir)+".code-workspace")

	// 1. Default init without --vscode does NOT create .code-workspace
	var stdout, stderr bytes.Buffer
	code := RunGlenCommand(ctx, []string{"init", emptyDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen init failed with code %d: %s", code, stderr.String())
	}
	if _, err := os.Stat(wsPath); !os.IsNotExist(err) {
		t.Fatalf("expected no .code-workspace without --vscode, got err=%v", err)
	}

	// 2. Running glen init --vscode on the existing Pure Bare repo (bare-repair) creates .code-workspace
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"init", "--vscode", emptyDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen init --vscode failed with code %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "VSCode File:") {
		t.Errorf("expected stdout to include VSCode File:, got:\n%s", stdout.String())
	}
	if _, err := os.Stat(wsPath); err != nil {
		t.Fatalf("expected .code-workspace to exist at %s: %v", wsPath, err)
	}

	checkFolders := func(wantNames ...string) {
		t.Helper()
		data, err := os.ReadFile(wsPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", wsPath, err)
		}
		var parsed struct {
			Folders []worktree.VSCodeFolder `json:"folders"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("failed to unmarshal %s: %v", wsPath, err)
		}
		if len(parsed.Folders) != len(wantNames) {
			t.Fatalf("folders count = %d (%+v), want %d (%v)", len(parsed.Folders), parsed.Folders, len(wantNames), wantNames)
		}
		for i, name := range wantNames {
			if parsed.Folders[i].Name != name || parsed.Folders[i].Path != name {
				t.Errorf("folder[%d] = %+v, want name/path %q", i, parsed.Folders[i], name)
			}
		}
	}

	checkFolders("main")

	// 3. glen create adds new worktree to .code-workspace
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"create", "feat/101-auth", "-C", emptyDir, "--fetch=false"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen create feat/101-auth failed: %s", stderr.String())
	}
	checkFolders("main", "feat/101-auth")

	// 4. glen create a second worktree, then delete its directory and run glen prune
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"create", "feat/102-prune-me", "-C", emptyDir, "--fetch=false"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen create feat/102-prune-me failed: %s", stderr.String())
	}
	checkFolders("main", "feat/101-auth", "feat/102-prune-me")

	if err := os.RemoveAll(filepath.Join(emptyDir, "feat", "102-prune-me")); err != nil {
		t.Fatalf("failed to remove feat/102-prune-me dir: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"prune", "-C", emptyDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen prune failed: %s", stderr.String())
	}
	checkFolders("main", "feat/101-auth")

	// 5. glen remove removes feat/101-auth from .code-workspace
	stdout.Reset()
	stderr.Reset()
	code = RunGlenCommand(ctx, []string{"remove", "feat/101-auth", "-C", emptyDir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("glen remove feat/101-auth failed: %s", stderr.String())
	}
	checkFolders("main")
}
