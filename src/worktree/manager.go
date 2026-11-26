package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"git-glen/internal/gitcfg"
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

// IsBareRepo checks if a path is a bare Git repository (and not a standard clone's .git directory).
func IsBareRepo(dir string) bool {
	clean := filepath.Clean(dir)
	if filepath.Base(clean) == ".git" {
		return false
	}
	if _, err := os.Stat(filepath.Join(clean, ".git")); err == nil {
		return false
	}
	head := filepath.Join(clean, "HEAD")
	cfg := filepath.Join(clean, "config")
	_, errHead := os.Stat(head)
	_, errCfg := os.Stat(cfg)
	return errHead == nil && errCfg == nil
}

func (m *Manager) resolveRepoConfig() (gitcfg.Repo, gitcfg.Config) {
	if m.RepoDir == "" {
		return gitcfg.Repo{}, gitcfg.LoadGlobal()
	}
	target := filepath.Clean(m.RepoDir)
	if filepath.Base(target) == ".git" {
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			target = filepath.Dir(target)
		}
	}
	repo, ok := gitcfg.Discover(target)
	if !ok {
		return gitcfg.Repo{BaseDir: target, CommonDir: m.RepoDir, GitDir: m.RepoDir}, gitcfg.LoadGlobal()
	}
	return repo, gitcfg.Load(repo)
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
		execDir := dir
		if filepath.Base(execDir) == ".git" {
			if info, err := os.Stat(execDir); err == nil && info.IsDir() {
				execDir = filepath.Dir(execDir)
			}
		}
		cmd = exec.CommandContext(ctx, "git", args...)
		cmd.Dir = execDir
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
// Git commands and IDEs recognize the bare workspace root as a Git repository.
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

func (m *Manager) applyErgonomicGitDefaults(ctx context.Context, ensureOriginRefspec bool) error {
	if ensureOriginRefspec {
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

// Governing: SPEC-0001, ADR-0003, REQ-0003
// EnsureBareRepoConfig ensures extensions.worktreeConfig, glen.layout=bare, per-worktree
// core.bare=true in .bare/config.worktree (with core.bare removed from shared .bare/config),
// remote.origin.fetch refspec, and ergonomic worktree defaults are configured for a bare repository.
func (m *Manager) EnsureBareRepoConfig(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// 1. Enable extensions.worktreeConfig first
	if err := m.EnsureWorktreeConfig(ctx); err != nil {
		return err
	}

	// 2. Remove core.bare from shared .bare/config (exit code 5 means key was not present)
	if _, err := m.runGit(ctx, m.RepoDir, "config", "--unset-all", "core.bare"); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 5 {
			return fmt.Errorf("failed to unset shared core.bare: %w", err)
		}
	}

	// 3. Set core.bare=true in .bare/config.worktree so root/.bare remains bare
	// without polluting linked worktrees created by plain `git worktree add`.
	if _, err := m.runGit(ctx, m.RepoDir, "config", "--worktree", "core.bare", "true"); err != nil {
		return fmt.Errorf("failed to set worktree core.bare=true: %w", err)
	}

	// 4. Record glen.layout=bare in shared .bare/config
	if _, err := m.runGit(ctx, m.RepoDir, "config", "glen.layout", string(LayoutBare)); err != nil {
		return fmt.Errorf("failed to set glen.layout=bare: %w", err)
	}

	return m.applyErgonomicGitDefaults(ctx, true)
}

// EnsureSubdirRepoConfig configures a standard clone for the subdir worktree layout:
// enables extensions.worktreeConfig, records glen.layout=subdir (and optional glen.worktreesDir),
// applies ergonomic Git worktree defaults, and registers the worktrees directory in .git/info/exclude.
func (m *Manager) EnsureSubdirRepoConfig(ctx context.Context, worktreesDir string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if err := m.EnsureWorktreeConfig(ctx); err != nil {
		return err
	}

	if _, err := m.runGit(ctx, m.RepoDir, "config", "glen.layout", string(LayoutSubdir)); err != nil {
		return fmt.Errorf("failed to set glen.layout=subdir: %w", err)
	}

	repo, cfg := m.resolveRepoConfig()
	effectiveWTDir := strings.TrimSpace(worktreesDir)
	if effectiveWTDir != "" {
		if _, err := m.runGit(ctx, m.RepoDir, "config", "glen.worktreesDir", effectiveWTDir); err != nil {
			return fmt.Errorf("failed to set glen.worktreesDir: %w", err)
		}
	} else {
		effectiveWTDir = cfg.WorktreesDir
	}

	hasOrigin := false
	if url, err := m.runGit(ctx, m.RepoDir, "config", "--get", "remote.origin.url"); err == nil && strings.TrimSpace(url) != "" {
		hasOrigin = true
	}
	if err := m.applyErgonomicGitDefaults(ctx, hasOrigin); err != nil {
		return err
	}

	strat := registeredLayouts[LayoutSubdir]
	wtRoot := strat.worktreesRoot(repo.BaseDir, effectiveWTDir)
	if pattern := strat.excludePattern(repo.BaseDir, wtRoot); pattern != "" && repo.CommonDir != "" {
		if err := gitcfg.EnsureInfoExclude(repo.CommonDir, pattern); err != nil {
			return fmt.Errorf("failed to update %s/info/exclude: %w", repo.CommonDir, err)
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
	_, _ = m.SyncVSCodeWorkspace(ctx, false)
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
// CreateWorktree creates an isolated worktree according to the repository's configured layout
// (<root>/<branch-name> for bare, <repo>/.worktrees/<branch-name> for subdir, or legacy sibling
// fallback when unconfigured) and configures optional per-worktree author identity.
func (m *Manager) CreateWorktree(ctx context.Context, branchName, baseBranch, agentName, agentEmail string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return "", "", fmt.Errorf("branch name is required")
	}

	isBare := IsBareRepo(m.RepoDir)
	repo, cfg := m.resolveRepoConfig()

	var worktreeDir string
	switch {
	case isBare || cfg.Layout == string(LayoutBare):
		if err := m.EnsureBareRepoConfig(ctx); err != nil {
			return "", "", err
		}
		rootDir := filepath.Clean(filepath.Join(m.RepoDir, ".."))
		_ = EnsureRootGitPointer(rootDir)
		strat := registeredLayouts[LayoutBare]
		worktreeDir = strat.worktreePath(strat.worktreesRoot(rootDir, ""), branchName)
	case cfg.Layout == string(LayoutSubdir):
		if err := m.EnsureSubdirRepoConfig(ctx, ""); err != nil {
			return "", "", err
		}
		strat := registeredLayouts[LayoutSubdir]
		wtRoot := strat.worktreesRoot(repo.BaseDir, cfg.WorktreesDir)
		worktreeDir = strat.worktreePath(wtRoot, branchName)
	default:
		if err := m.EnsureWorktreeConfig(ctx); err != nil {
			return "", "", err
		}
		dirSlug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(branchName, "/", "-"), " ", "-"))
		worktreeDir = filepath.Clean(filepath.Join(m.RepoDir, "..", "worktrees", dirSlug))
	}

	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0755); err != nil {
		return "", "", fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	defBranch := cfg.DefaultBranch
	if defBranch == "" {
		defBranch = "main"
	}
	defaultBase := baseBranch == ""
	if defaultBase {
		baseBranch = "origin/" + defBranch
	}

	_, err := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, baseBranch)
	if err != nil && defaultBase {
		// Fallback to local default branch or HEAD if remote tracking branch is not available
		if _, errDef := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, defBranch); errDef == nil {
			err = nil
		} else if defBranch != "main" {
			if _, errMain := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, "main"); errMain == nil {
				err = nil
			}
		}
		if err != nil {
			if _, errHead := m.runGit(ctx, m.RepoDir, "worktree", "add", worktreeDir, "-b", branchName, "HEAD"); errHead == nil {
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
	if isBare || cfg.Layout == string(LayoutBare) {
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

	_, _ = m.SyncVSCodeWorkspace(ctx, false)

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

	isBare := IsBareRepo(m.RepoDir)
	repo, cfg := m.resolveRepoConfig()

	// Remove worktree
	_, err := m.runGit(ctx, m.RepoDir, "worktree", "remove", "--force", worktreeDir)
	if err != nil {
		// If git worktree remove fails, remove directory and prune
		_ = os.RemoveAll(worktreeDir)
	}
	// Defensively prune stale metadata and recover locks
	_ = m.PruneWorktrees(ctx)

	// Clean up empty intermediate branch directories (e.g., <wtRoot>/feat/) up to the worktrees root.
	var stopDir string
	switch {
	case isBare || cfg.Layout == string(LayoutBare):
		stopDir = filepath.Clean(filepath.Join(m.RepoDir, ".."))
	case cfg.Layout == string(LayoutSubdir):
		stopDir = registeredLayouts[LayoutSubdir].worktreesRoot(repo.BaseDir, cfg.WorktreesDir)
	}
	if stopDir != "" {
		parent := filepath.Dir(filepath.Clean(worktreeDir))
		for parent != stopDir && parent != "." && parent != string(filepath.Separator) && strings.HasPrefix(parent, stopDir+string(filepath.Separator)) {
			if err := os.Remove(parent); err != nil {
				break
			}
			parent = filepath.Dir(parent)
		}
	}
	_, _ = m.SyncVSCodeWorkspace(ctx, false)

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
