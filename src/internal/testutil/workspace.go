package testutil

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const gitCommandTimeout = 30 * time.Second

// RunGit executes a git command in the specified directory with a defensive outer timeout.
// It returns the trimmed stdout output, or fails the test with detailed diagnostics if the command fails.
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return RunGitEnv(t, dir, nil, args...)
}

// isBareRepo checks if dir is a bare git directory without .git.
func isBareRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return false
	}
	head := filepath.Join(dir, "HEAD")
	cfg := filepath.Join(dir, "config")
	_, errHead := os.Stat(head)
	_, errCfg := os.Stat(cfg)
	return errHead == nil && errCfg == nil
}

// RunGitEnv executes a git command in the specified directory with custom environment variables and a defensive timeout.
func RunGitEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if dir != "" && isBareRepo(dir) {
		cmd = exec.CommandContext(ctx, "git", append([]string{"--git-dir=" + dir}, args...)...)
		cmd.Dir = dir
	} else {
		cmd = exec.CommandContext(ctx, "git", args...)
		if dir != "" {
			cmd.Dir = dir
		}
	}

	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if len(env) > 0 {
		cmd.Env = append(cmd.Env, env...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("git command failed: git %s\ndir: %s\nerr: %v\nstderr:\n%s\nstdout:\n%s",
			strings.Join(args, " "), dir, err, strings.TrimSpace(stderr.String()), strings.TrimSpace(stdout.String()))
	}
	return strings.TrimSpace(stdout.String())
}

// fixPermissions ensures all files and directories under root are writable so t.TempDir() cleanup succeeds.
func fixPermissions(root string) {
	cmd := exec.Command("chmod", "-R", "u+rwx", root)
	_ = cmd.Run()
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			_ = os.Chmod(path, 0777)
		}
		return nil
	})
}

// SetupPureBareWorkspace creates a temporary .bare + .git + main/ hierarchy inside t.TempDir().
// It returns rootDir, bareDir (.bare), and mainDir (main worktree).
func SetupPureBareWorkspace(t *testing.T) (rootDir, bareDir, mainDir string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	rootDir = t.TempDir()
	t.Cleanup(func() { fixPermissions(rootDir) })

	bareDir = filepath.Join(rootDir, ".bare")
	mainDir = filepath.Join(rootDir, "main")

	RunGit(t, rootDir, "init", "--bare", ".bare")
	if err := os.WriteFile(filepath.Join(rootDir, ".git"), []byte("gitdir: ./.bare\n"), 0644); err != nil {
		t.Fatalf("failed to write root .git pointer: %v", err)
	}
	RunGit(t, bareDir, "config", "extensions.worktreeConfig", "true")
	RunGit(t, bareDir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	RunGit(t, bareDir, "config", "push.autoSetupRemote", "true")
	RunGit(t, bareDir, "config", "branch.autoSetupMerge", "simple")
	RunGit(t, bareDir, "config", "fetch.prune", "true")
	RunGit(t, bareDir, "config", "worktree.guessRemote", "true")

	RunGit(t, bareDir, "worktree", "add", mainDir, "-b", "main")

	// Configure main worktree
	RunGit(t, mainDir, "config", "--worktree", "core.bare", "false")
	RunGit(t, mainDir, "config", "--worktree", "user.name", "Test User")
	RunGit(t, mainDir, "config", "--worktree", "user.email", "test@example.com")

	// Commit initial README in main
	readmePath := filepath.Join(mainDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Pure Bare Test Workspace\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md in main worktree: %v", err)
	}
	RunGit(t, mainDir, "add", "README.md")
	RunGit(t, mainDir, "commit", "-m", "Initial commit on main")

	return rootDir, bareDir, mainDir
}

// SetupStandardClone creates a standard clean clone directory containing .git/.
// If dirty is true, it introduces uncommitted modifications and untracked files.
func SetupStandardClone(t *testing.T, dirty bool) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	cloneDir := t.TempDir()
	t.Cleanup(func() { fixPermissions(cloneDir) })

	RunGit(t, cloneDir, "init", "-b", "main")
	RunGit(t, cloneDir, "config", "user.name", "Test User")
	RunGit(t, cloneDir, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(cloneDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Standard Clone Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	RunGit(t, cloneDir, "add", "README.md")
	RunGit(t, cloneDir, "commit", "-m", "Initial commit")

	if dirty {
		scratchPath := filepath.Join(cloneDir, "scratch.txt")
		if err := os.WriteFile(scratchPath, []byte("untracked content\n"), 0644); err != nil {
			t.Fatalf("failed to write scratch.txt: %v", err)
		}
		if err := os.WriteFile(readmePath, []byte("# Standard Clone Test (Modified)\n"), 0644); err != nil {
			t.Fatalf("failed to modify README.md: %v", err)
		}
	}

	return cloneDir
}

// SetupDirtyClone creates a standard clone containing untracked and modified files.
func SetupDirtyClone(t *testing.T) string {
	t.Helper()
	return SetupStandardClone(t, true)
}

// SetupLegacySiblingLayout creates a root directory containing repo/ (standard clone) and worktrees/.
func SetupLegacySiblingLayout(t *testing.T) (rootDir, repoDir, worktreesDir string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	rootDir = t.TempDir()
	t.Cleanup(func() { fixPermissions(rootDir) })

	repoDir = filepath.Join(rootDir, "repo")
	worktreesDir = filepath.Join(rootDir, "worktrees")

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo directory: %v", err)
	}

	RunGit(t, repoDir, "init", "-b", "main")
	RunGit(t, repoDir, "config", "user.name", "Test User")
	RunGit(t, repoDir, "config", "user.email", "test@example.com")

	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Legacy Sibling Test\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	RunGit(t, repoDir, "add", "README.md")
	RunGit(t, repoDir, "commit", "-m", "Initial commit in legacy repo")

	if err := os.MkdirAll(worktreesDir, 0755); err != nil {
		t.Fatalf("failed to create worktrees directory: %v", err)
	}

	return rootDir, repoDir, worktreesDir
}

// SetupGreenfieldDirectory creates an empty temporary directory.
func SetupGreenfieldDirectory(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return t.TempDir()
}

// CreateFeatureWorktreeInWorkspace creates an isolated feature worktree at <root>/<branchName>
// linked to bareDir and branching from "main".
func CreateFeatureWorktreeInWorkspace(t *testing.T, bareDir, wtName, branchName string) string {
	t.Helper()
	rootDir := filepath.Dir(bareDir)
	relPath := branchName
	if relPath == "" {
		relPath = wtName
	}
	wtDir := filepath.Join(rootDir, filepath.FromSlash(relPath))

	if err := os.MkdirAll(filepath.Dir(wtDir), 0755); err != nil {
		t.Fatalf("failed to create worktree parent directory: %v", err)
	}

	RunGit(t, bareDir, "worktree", "add", wtDir, "-b", branchName, "main")
	RunGit(t, wtDir, "config", "--worktree", "core.bare", "false")
	RunGit(t, wtDir, "config", "--worktree", "user.name", "Feature Agent")
	RunGit(t, wtDir, "config", "--worktree", "user.email", "agent@example.com")

	return wtDir
}
