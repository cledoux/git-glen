package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultTimeout = 30 * time.Second

// Manager manages ephemeral Git worktrees with isolated configurations.
type Manager struct {
	RepoDir string
}

// NewManager creates a new worktree Manager.
func NewManager(repoDir string) *Manager {
	return &Manager{RepoDir: repoDir}
}

// WorktreeInfo contains metadata about an active Git worktree.
type WorktreeInfo struct {
	Path   string
	Head   string
	Branch string
	Bare   bool
}

// IsBareRepo checks if a path is a bare Git repository.
func IsBareRepo(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return false
	}
	head := filepath.Join(dir, "HEAD")
	cfg := filepath.Join(dir, "config")
	_, errHead := os.Stat(head)
	_, errCfg := os.Stat(cfg)
	return errHead == nil && errCfg == nil
}

// runGit executes a git command in the repo directory with a timeout.
func (m *Manager) runGit(ctx context.Context, dir string, args ...string) (string, error) {
	if dir == "" {
		dir = m.RepoDir
	}
	var cmd *exec.Cmd
	if dir == m.RepoDir && IsBareRepo(m.RepoDir) {
		cmd = exec.CommandContext(ctx, "git", append([]string{"--git-dir=" + m.RepoDir}, args...)...)
		cmd.Dir = m.RepoDir
	} else {
		cmd = exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// EnsureWorktreeConfig enables extensions.worktreeConfig in the repository.
func (m *Manager) EnsureWorktreeConfig(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := m.runGit(ctx, m.RepoDir, "config", "extensions.worktreeConfig", "true")
	if err != nil {
		return fmt.Errorf("failed to enable extensions.worktreeConfig: %w", err)
	}
	return nil
}

// EnsureRootGitPointer writes the root .git pointer file ("gitdir: ./.bare\n") so
// Git commands and IDEs recognize the Pure Bare workspace root as a Git repository.
func EnsureRootGitPointer(rootDir string) error {
	gitFile := filepath.Join(rootDir, ".git")
	if info, err := os.Stat(gitFile); err == nil && info.IsDir() {
		return fmt.Errorf("cannot write root .git pointer: %s is a directory", gitFile)
	}
	if err := os.WriteFile(gitFile, []byte("gitdir: ./.bare\n"), 0644); err != nil {
		return fmt.Errorf("failed to write root .git pointer at %s: %w", gitFile, err)
	}
	return nil
}

// Governing: SPEC-0001, ADR-0003, REQ-0003
// EnsureBareRepoConfig ensures extensions.worktreeConfig, remote.origin.fetch refspec,
// and ergonomic bare worktree defaults (push.autoSetupRemote, branch.autoSetupMerge,
// fetch.prune, worktree.guessRemote) are configured for a bare repository.
func (m *Manager) EnsureBareRepoConfig(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if err := m.EnsureWorktreeConfig(ctx); err != nil {
		return err
	}

	// Check existing remote.origin.fetch refspecs
	existing, _ := m.runGit(ctx, m.RepoDir, "config", "--get-all", "remote.origin.fetch")
	expectedRefspec := "+refs/heads/*:refs/remotes/origin/*"
	hasRefspec := false
	for _, line := range strings.Split(existing, "\n") {
		if strings.TrimSpace(line) == expectedRefspec {
			hasRefspec = true
			break
		}
	}

	if !hasRefspec {
		if _, err := m.runGit(ctx, m.RepoDir, "config", "remote.origin.fetch", expectedRefspec); err != nil {
			return fmt.Errorf("failed to set remote.origin.fetch: %w", err)
		}
	}

	defaultConfigs := []struct {
		key string
		val string
	}{
		{"push.autoSetupRemote", "true"},
		{"branch.autoSetupMerge", "simple"},
		{"fetch.prune", "true"},
		{"worktree.guessRemote", "true"},
	}
	for _, kv := range defaultConfigs {
		if _, err := m.runGit(ctx, m.RepoDir, "config", kv.key, kv.val); err != nil {
			return fmt.Errorf("failed to set %s=%s: %w", kv.key, kv.val, err)
		}
	}

	return nil
}

// Governing: SPEC-0001, ADR-0003, REQ-0004
// PruneWorktrees runs git worktree prune --expire now to clear stale metadata and recover locks.
func (m *Manager) PruneWorktrees(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := m.runGit(ctx, m.RepoDir, "worktree", "prune", "--expire", "now")
	if err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}
	return nil
}

// FetchRemote fetches latest refs from the specified remote if remote.<name>.url is configured.
func (m *Manager) FetchRemote(ctx context.Context, remote string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if remote == "" {
		remote = "origin"
	}
	if url, err := m.runGit(ctx, m.RepoDir, "config", "--get", fmt.Sprintf("remote.%s.url", remote)); err != nil || strings.TrimSpace(url) == "" {
		return nil
	}
	_, err := m.runGit(ctx, m.RepoDir, "fetch", remote)
	return err
}

// Governing: SPEC-0001, ADR-0003, REQ-0003, REQ-0004
// CreateWorktree creates an isolated worktree at <root>/<branch-name> (for Pure Bare repositories)
// or worktrees/<branch-slug> (for legacy non-bare setups) and configures core.bare=false and
// optional per-worktree author identity.
func (m *Manager) CreateWorktree(ctx context.Context, branchName, baseBranch, agentName, agentEmail string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return "", "", fmt.Errorf("branch name is required")
	}

	isBare := IsBareRepo(m.RepoDir)
	var worktreeDir string
	if isBare {
		if err := m.EnsureBareRepoConfig(ctx); err != nil {
			return "", "", err
		}
		rootDir := filepath.Clean(filepath.Join(m.RepoDir, ".."))
		_ = EnsureRootGitPointer(rootDir)
		cleanBranch := strings.Trim(strings.ReplaceAll(branchName, " ", "-"), "/")
		worktreeDir = filepath.Clean(filepath.Join(rootDir, filepath.FromSlash(cleanBranch)))
	} else {
		if err := m.EnsureWorktreeConfig(ctx); err != nil {
			return "", "", err
		}
		dirSlug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(branchName, "/", "-"), " ", "-"))
		worktreeDir = filepath.Clean(filepath.Join(m.RepoDir, "..", "worktrees", dirSlug))
	}

	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0755); err != nil {
		return "", "", fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	if baseBranch == "" {
		baseBranch = "origin/main"
	}
	_, err := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, baseBranch)
	if err != nil {
		// Fallback to local default branch or HEAD if remote branch is not available
		if baseBranch == "origin/main" {
			if _, errMain := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, "main"); errMain == nil {
				err = nil
			} else if _, errHead := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, "HEAD"); errHead == nil {
				err = nil
			}
		}
	}
	if err != nil {
		// If branch already exists locally, try checking it out directly
		_, errRetry := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, branchName)
		if errRetry != nil {
			return "", "", fmt.Errorf("failed to add git worktree: %w", err)
		}
	}

	// When attached to a bare repository database, configure the worktree as non-bare
	if isBare {
		if _, err := m.runGit(ctx, worktreeDir, "config", "--worktree", "core.bare", "false"); err != nil {
			_ = os.RemoveAll(worktreeDir)
			return "", "", fmt.Errorf("failed to configure worktree core.bare: %w", err)
		}
	}

	// Configure per-worktree author identity
	if agentName != "" {
		if _, err := m.runGit(ctx, worktreeDir, "config", "--worktree", "user.name", agentName); err != nil {
			return "", "", fmt.Errorf("failed to set worktree user.name: %w", err)
		}
	}
	if agentEmail != "" {
		if _, err := m.runGit(ctx, worktreeDir, "config", "--worktree", "user.email", agentEmail); err != nil {
			return "", "", fmt.Errorf("failed to set worktree user.email: %w", err)
		}
	}

	return worktreeDir, branchName, nil
}

// Governing: SPEC-0001, ADR-0003, REQ-0003, REQ-0004
// CreateFeatureWorktree creates an ephemeral worktree for an issue and configures isolated author identity.
func (m *Manager) CreateFeatureWorktree(ctx context.Context, issueID int, slug, baseBranch, agentName, agentEmail string) (string, string, error) {
	cleanSlug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(slug), " ", "-"))
	if cleanSlug == "" {
		cleanSlug = "task"
	}
	branchName := fmt.Sprintf("feat/%d-%s", issueID, cleanSlug)
	return m.CreateWorktree(ctx, branchName, baseBranch, agentName, agentEmail)
}

// Governing: SPEC-0001, ADR-0003, REQ-0004
// RemoveFeatureWorktree removes an ephemeral worktree, cleans up empty parent branch folders,
// and deletes the local feature branch.
func (m *Manager) RemoveFeatureWorktree(ctx context.Context, worktreeDir, branchName string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Remove worktree
	_, err := m.runGit(ctx, m.RepoDir, "worktree", "remove", "--force", worktreeDir)
	if err != nil {
		// If git worktree remove fails, remove directory and prune
		_ = os.RemoveAll(worktreeDir)
	}
	// Defensively prune stale metadata and recover locks
	_ = m.PruneWorktrees(ctx)

	// Clean up empty intermediate branch directories (e.g., <root>/feat/) in Pure Bare workspaces
	if IsBareRepo(m.RepoDir) {
		rootDir := filepath.Clean(filepath.Join(m.RepoDir, ".."))
		parent := filepath.Dir(filepath.Clean(worktreeDir))
		for parent != rootDir && parent != "." && parent != string(filepath.Separator) && strings.HasPrefix(parent, rootDir+string(filepath.Separator)) {
			if err := os.Remove(parent); err != nil {
				break
			}
			parent = filepath.Dir(parent)
		}
	}

	// Delete local feature branch
	if branchName != "" {
		_, _ = m.runGit(ctx, m.RepoDir, "branch", "-D", branchName)
	}

	return nil
}

// ListWorktrees returns all active worktrees for the repository.
func (m *Manager) ListWorktrees(ctx context.Context) ([]WorktreeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	out, err := m.runGit(ctx, m.RepoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	var results []WorktreeInfo
	var current WorktreeInfo
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if current.Path != "" {
				results = append(results, current)
				current = WorktreeInfo{}
			}
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		switch parts[0] {
		case "worktree":
			current.Path = parts[1]
		case "HEAD":
			current.Head = parts[1]
		case "branch":
			current.Branch = strings.TrimPrefix(parts[1], "refs/heads/")
		case "bare":
			current.Bare = true
		}
	}
	if current.Path != "" {
		results = append(results, current)
	}

	return results, nil
}
