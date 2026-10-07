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

// Scenario represents the detected repository initialization scenario.
type Scenario string

const (
	ScenarioRemoteClone   Scenario = "remote-clone"
	ScenarioCloneConvert  Scenario = "clone-convert"
	ScenarioBareRepair    Scenario = "bare-repair"
	ScenarioLegacyMigrate Scenario = "legacy-migrate"
	ScenarioGreenfield    Scenario = "greenfield"
)

// PureBareGuardMarker identifies the automated Glen guardrail block.
const PureBareGuardMarker = "<!-- glen:pure-bare-guard -->"

// PureBareGuardSnippet defines the lightweight tripwire injected into workspace rules.
const PureBareGuardSnippet = "<!-- glen:pure-bare-guard -->\n## Pure Bare Worktree Discipline\nThis repository uses the Pure Bare topology (`.bare/` + `main/` + `<branch>/`).\n- NEVER edit feature or bugfix code directly in `main/`.\n- NEVER run working-tree Git commands at the workspace root.\n- ALWAYS use `glen create <branch-name>` or the `worktree` skill before modifying code.\n"

// InitResult contains execution summary metadata from repository initialization.
type InitResult struct {
	Scenario      Scenario
	BaseDir       string
	BareDir       string
	MainDir       string
	DefaultBranch string
	Repaired      bool
	Migrated      bool
	Converted     bool
}

// DirtyTreeError indicates that a repository working copy contains uncommitted or untracked files.
type DirtyTreeError struct {
	Dir   string
	Files []string
}

func (e *DirtyTreeError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("working directory '%s' contains uncommitted changes or untracked files:\n", e.Dir))
	for _, f := range e.Files {
		sb.WriteString(fmt.Sprintf("  %s\n", f))
	}
	sb.WriteString("\nSuggested actions:\n")
	sb.WriteString(fmt.Sprintf("  1. Commit changes:   git -C %s add -A && git -C %s commit -m \"WIP\"\n", e.Dir, e.Dir))
	sb.WriteString(fmt.Sprintf("  2. Or stash changes: git -C %s stash --include-untracked\n", e.Dir))
	sb.WriteString("  3. Re-run initialization")
	return sb.String()
}

// CheckWorkingTreeClean inspects a working directory using git status --porcelain.
// Spec: SPEC-0002 (REQ-0003), SPEC-0001 (REQ-0012)
func CheckWorkingTreeClean(ctx context.Context, dir string) (bool, []string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, nil, fmt.Errorf("failed to check working tree status in %s: %w (stderr: %s)", dir, err, strings.TrimSpace(stderr.String()))
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return true, nil, nil
	}

	lines := strings.Split(output, "\n")
	var dirtyFiles []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			dirtyFiles = append(dirtyFiles, trimmed)
		}
	}
	return false, dirtyFiles, nil
}

// DetectScenario inspects target and classifies the initialization scenario.
func DetectScenario(target string) (Scenario, string, error) {
	target = strings.TrimSpace(target)
	if target == "" || target == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("failed to get current working directory: %w", err)
		}
		target = cwd
	}

	// 1. Check if target is a remote Git URL
	if isRemoteURL(target) {
		repoName := extractRepoNameFromURL(target)
		return ScenarioRemoteClone, repoName, nil
	}

	absPath, err := filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve absolute path for %s: %w", target, err)
	}

	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return ScenarioGreenfield, absPath, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to inspect target path %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("target %s is not a directory", absPath)
	}

	// 2. Check if target has .bare/ directory (Pure Bare workspace)
	bareDir := filepath.Join(absPath, ".bare")
	if IsBareRepo(bareDir) || isBareDirPresent(bareDir) {
		return ScenarioBareRepair, absPath, nil
	}

	// 3. Check if target has legacy repo/.git layout
	legacyRepoGit := filepath.Join(absPath, "repo", ".git")
	if info, err := os.Stat(legacyRepoGit); err == nil && (info.IsDir() || !info.IsDir()) {
		return ScenarioLegacyMigrate, absPath, nil
	}

	// 4. Check if target has .git directory (Standard Clone)
	gitPath := filepath.Join(absPath, ".git")
	if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
		return ScenarioCloneConvert, absPath, nil
	}

	// 5. Check if directory is empty (Greenfield)
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read directory %s: %w", absPath, err)
	}
	if len(entries) == 0 {
		return ScenarioGreenfield, absPath, nil
	}

	return "", "", fmt.Errorf("target directory %s is non-empty and not a recognized Git repository", absPath)
}

func isRemoteURL(target string) bool {
	if strings.HasPrefix(target, "http://") ||
		strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "git@") ||
		strings.HasPrefix(target, "ssh://") {
		return true
	}
	if strings.HasSuffix(target, ".git") && !strings.Contains(target, string(filepath.Separator)) {
		return true
	}
	return false
}

func extractRepoNameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if colonIdx := strings.LastIndex(last, ":"); colonIdx != -1 {
			last = last[colonIdx+1:]
		}
		return strings.TrimSpace(last)
	}
	return "project"
}

func isBareDirPresent(dir string) bool {
	head := filepath.Join(dir, "HEAD")
	cfg := filepath.Join(dir, "config")
	_, errHead := os.Stat(head)
	_, errCfg := os.Stat(cfg)
	return errHead == nil && errCfg == nil
}

// ProbeDefaultBranch dynamically resolves the default branch name (e.g. main, master, trunk).
func ProbeDefaultBranch(ctx context.Context, bareDir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Try remote symbolic-ref
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		ref := strings.TrimSpace(stdout.String())
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/"), nil
		}
	}

	// Try local symbolic-ref
	stdout.Reset()
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "symbolic-ref", "HEAD")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		ref := strings.TrimSpace(stdout.String())
		if strings.HasPrefix(ref, "refs/heads/") {
			return strings.TrimPrefix(ref, "refs/heads/"), nil
		}
	}

	// Probe origin/main or origin/master
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "rev-parse", "--verify", "origin/main")
	if err := cmd.Run(); err == nil {
		return "main", nil
	}
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "rev-parse", "--verify", "origin/master")
	if err := cmd.Run(); err == nil {
		return "master", nil
	}

	// Default fallback
	return "main", nil
}

// Init delegates execution to the appropriate initialization workflow based on target detection.
func (m *Manager) Init(ctx context.Context, target string) (*InitResult, error) {
	scen, resolvedPath, err := DetectScenario(target)
	if err != nil {
		return nil, err
	}

	switch scen {
	case ScenarioRemoteClone:
		return m.InitBareRepo(ctx, target, resolvedPath)
	case ScenarioCloneConvert:
		return m.ConvertStandardClone(ctx, resolvedPath)
	case ScenarioBareRepair:
		return m.RepairWorkspace(ctx, resolvedPath)
	case ScenarioLegacyMigrate:
		return m.MigrateLegacySibling(ctx, resolvedPath)
	case ScenarioGreenfield:
		return m.InitGreenfield(ctx, resolvedPath)
	default:
		return nil, fmt.Errorf("unsupported initialization scenario: %s", scen)
	}
}

// InitBareRepo clones a remote repository into a Pure Bare root topology (.bare/ + .git + main/).
// Spec: SPEC-0002 (REQ-0002), SPEC-0001 (REQ-0011)
func (m *Manager) InitBareRepo(ctx context.Context, repoURL, targetDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if targetDir == "" {
		targetDir = extractRepoNameFromURL(repoURL)
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for %s: %w", targetDir, err)
	}

	if info, err := os.Stat(absTarget); err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("target %s is a file, not a directory", absTarget)
		}
		entries, _ := os.ReadDir(absTarget)
		if len(entries) > 0 {
			// If already a bare workspace, repair it
			if isBareDirPresent(filepath.Join(absTarget, ".bare")) {
				return m.RepairWorkspace(ctx, absTarget)
			}
			return nil, fmt.Errorf("target directory %s already exists and is not empty", absTarget)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(absTarget, 0755); err != nil {
			return nil, fmt.Errorf("failed to create target directory %s: %w", absTarget, err)
		}
	}

	bareDir := filepath.Join(absTarget, ".bare")
	mainDir := filepath.Join(absTarget, "main")

	// 1. Bare clone
	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", repoURL, bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to clone bare repository: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure bare repository: %w", err)
	}
	if err := EnsureRootGitPointer(absTarget); err != nil {
		return nil, err
	}

	// 2. Probe default branch
	branch, _ := ProbeDefaultBranch(ctx, bareDir)
	if branch == "" {
		branch = "main"
	}

	// 3. Attach primary main worktree
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, "origin/"+branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try without origin/ prefix if origin branch is local
		cmdRetry := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, branch)
		if outRetry, errRetry := cmdRetry.CombinedOutput(); errRetry != nil {
			return nil, fmt.Errorf("failed to add main worktree: %w (output: %s / retry: %s)", err, strings.TrimSpace(string(out)), strings.TrimSpace(string(outRetry)))
		}
	}

	// 4. Configure main worktree
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false")
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "branch", "--set-upstream-to=origin/"+branch, branch)
	_ = cmd.Run()

	_ = EnsureWorkspaceGuard(absTarget)

	return &InitResult{
		Scenario:      ScenarioRemoteClone,
		BaseDir:       absTarget,
		BareDir:       bareDir,
		MainDir:       mainDir,
		DefaultBranch: branch,
	}, nil
}

// ConvertStandardClone transforms an existing clean standard clone into Pure Bare topology.
// Spec: SPEC-0002 (REQ-0002, REQ-0003), SPEC-0001 (REQ-0011, REQ-0012)
func (m *Manager) ConvertStandardClone(ctx context.Context, cloneDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absClone, err := filepath.Abs(cloneDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", cloneDir, err)
	}

	// 1. Mandatory Cleanliness Check
	clean, dirtyFiles, err := CheckWorkingTreeClean(ctx, absClone)
	if err != nil {
		return nil, err
	}
	if !clean {
		return nil, &DirtyTreeError{Dir: absClone, Files: dirtyFiles}
	}

	// 2. Resolve active branch name
	cmd := exec.CommandContext(ctx, "git", "-C", absClone, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	branch := "main"
	if err := cmd.Run(); err == nil {
		b := strings.TrimSpace(stdout.String())
		if b != "" && b != "HEAD" {
			branch = b
		}
	}

	// 3. Move .git to .bare
	oldGitDir := filepath.Join(absClone, ".git")
	bareDir := filepath.Join(absClone, ".bare")
	if err := os.Rename(oldGitDir, bareDir); err != nil {
		return nil, fmt.Errorf("failed to move .git to .bare: %w", err)
	}

	// 4. Configure bare database
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "config", "core.bare", "true")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to set core.bare true: %w (%s)", err, out)
	}

	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure bare repository: %w", err)
	}

	// 5. Clean up old working tree files from root (since tree is clean and checked into git history)
	entries, err := os.ReadDir(absClone)
	if err != nil {
		return nil, fmt.Errorf("failed to list files in %s: %w", absClone, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".bare" {
			continue
		}
		_ = os.RemoveAll(filepath.Join(absClone, name))
	}

	// Write root .git pointer file ("gitdir: ./.bare")
	if err := EnsureRootGitPointer(absClone); err != nil {
		return nil, err
	}

	// 6. Add primary main worktree
	mainDir := filepath.Join(absClone, "main")
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to add main worktree: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	// 7. Configure main worktree core.bare false
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false")
	_ = cmd.Run()

	_ = EnsureWorkspaceGuard(absClone)

	return &InitResult{
		Scenario:      ScenarioCloneConvert,
		BaseDir:       absClone,
		BareDir:       bareDir,
		MainDir:       mainDir,
		DefaultBranch: branch,
		Converted:     true,
	}, nil
}

// RepairWorkspace repairs worktree pointers, enforces configs, and prunes stale locks in a Pure Bare repository.
// Spec: SPEC-0002 (REQ-0002), SPEC-0001 (REQ-0013)
func (m *Manager) RepairWorkspace(ctx context.Context, rootDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", rootDir, err)
	}

	bareDir := filepath.Join(absRoot, ".bare")
	mainDir := filepath.Join(absRoot, "main")

	if !isBareDirPresent(bareDir) {
		return nil, fmt.Errorf("cannot repair workspace: .bare directory not found in %s", absRoot)
	}

	if err := EnsureRootGitPointer(absRoot); err != nil {
		return nil, err
	}

	// 1. Repair worktrees
	if _, err := os.Stat(mainDir); err == nil {
		cmd := exec.CommandContext(ctx, "git", "-C", mainDir, "worktree", "repair")
		_ = cmd.Run()
	}
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "repair")
	_ = cmd.Run()

	// 2. Enforce bare configuration & fetch refspecs
	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to enforce bare repo config: %w", err)
	}

	// 3. Defensive prune to recover locks
	if err := bareMgr.PruneWorktrees(ctx); err != nil {
		return nil, fmt.Errorf("failed to prune worktrees: %w", err)
	}

	// Remove empty legacy worktrees/ container if present
	_ = os.Remove(filepath.Join(absRoot, "worktrees"))

	branch, _ := ProbeDefaultBranch(ctx, bareDir)
	if branch == "" {
		branch = "main"
	}

	_ = EnsureWorkspaceGuard(absRoot)

	return &InitResult{
		Scenario:      ScenarioBareRepair,
		BaseDir:       absRoot,
		BareDir:       bareDir,
		MainDir:       mainDir,
		DefaultBranch: branch,
		Repaired:      true,
	}, nil
}

// MigrateLegacySibling converts a legacy sibling layout (repo/ + worktrees/) into Pure Bare topology.
// Spec: SPEC-0002 (REQ-0002), SPEC-0001 (REQ-0011)
func (m *Manager) MigrateLegacySibling(ctx context.Context, rootDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", rootDir, err)
	}

	repoDir := filepath.Join(absRoot, "repo")
	legacyGit := filepath.Join(repoDir, ".git")

	if _, err := os.Stat(legacyGit); err != nil {
		return nil, fmt.Errorf("legacy repo/.git not found in %s: %w", absRoot, err)
	}

	// 1. Mandatory Cleanliness Check on legacy repo
	clean, dirtyFiles, err := CheckWorkingTreeClean(ctx, repoDir)
	if err != nil {
		return nil, err
	}
	if !clean {
		return nil, &DirtyTreeError{Dir: repoDir, Files: dirtyFiles}
	}

	// 2. Resolve branch
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	branch := "main"
	if err := cmd.Run(); err == nil {
		b := strings.TrimSpace(stdout.String())
		if b != "" && b != "HEAD" {
			branch = b
		}
	}

	// 3. Move repo/.git to .bare
	bareDir := filepath.Join(absRoot, ".bare")
	if err := os.Rename(legacyGit, bareDir); err != nil {
		return nil, fmt.Errorf("failed to move repo/.git to .bare: %w", err)
	}

	// 4. Configure bare
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "config", "core.bare", "true")
	_ = cmd.Run()

	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure bare repository: %w", err)
	}

	// 5. Remove legacy repo working directory
	_ = os.RemoveAll(repoDir)

	if err := EnsureRootGitPointer(absRoot); err != nil {
		return nil, err
	}

	// 6. Add primary main worktree
	mainDir := filepath.Join(absRoot, "main")
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to add main worktree: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	// 7. Configure main worktree
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false")
	_ = cmd.Run()

	// 8. Defensive prune
	_ = bareMgr.PruneWorktrees(ctx)

	// 9. Remove empty legacy worktrees container if unused
	_ = os.Remove(filepath.Join(absRoot, "worktrees"))

	_ = EnsureWorkspaceGuard(absRoot)

	return &InitResult{
		Scenario:      ScenarioLegacyMigrate,
		BaseDir:       absRoot,
		BareDir:       bareDir,
		MainDir:       mainDir,
		DefaultBranch: branch,
		Migrated:      true,
	}, nil
}

// InitGreenfield initializes a fresh empty directory into a Pure Bare repository.
// Spec: SPEC-0002 (REQ-0002), SPEC-0001 (REQ-0011)
func (m *Manager) InitGreenfield(ctx context.Context, targetDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", targetDir, err)
	}

	if err := os.MkdirAll(absTarget, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory %s: %w", absTarget, err)
	}

	bareDir := filepath.Join(absTarget, ".bare")
	mainDir := filepath.Join(absTarget, "main")

	// 1. Init bare database
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", "-b", "main", bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to git init bare: %w (%s)", err, out)
	}

	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure bare repository: %w", err)
	}
	if err := EnsureRootGitPointer(absTarget); err != nil {
		return nil, err
	}

	// 2. Add main worktree
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, "-b", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to add main worktree: %w (%s)", err, out)
	}

	// 3. Configure main worktree
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false")
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "user.name", "Agent Teams")
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "user.email", "agent@example.com")
	_ = cmd.Run()

	// 4. Initial commit in main
	readmePath := filepath.Join(mainDir, "README.md")
	_ = os.WriteFile(readmePath, []byte("# Project\n\nInitialized with `glen init`.\n"), 0644)
	_ = EnsureWorkspaceGuard(absTarget)
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "add", "-A")
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "commit", "-m", "Initial commit")
	_ = cmd.Run()

	return &InitResult{
		Scenario:      ScenarioGreenfield,
		BaseDir:       absTarget,
		BareDir:       bareDir,
		MainDir:       mainDir,
		DefaultBranch: "main",
	}, nil
}

// EnsureWorkspaceGuard ensures that rootDir (the Pure Bare workspace root) contains a
// workspace GEMINI.md (or AGENTS.md) with the Pure Bare guardrail tripwire.
// Note: This file is written strictly to rootDir (outside of main/ or any working tree),
// ensuring it is never tracked, staged, or committed to the repository.
func EnsureWorkspaceGuard(rootDir string) error {
	if rootDir == "" {
		return nil
	}
	if info, err := os.Stat(rootDir); err != nil || !info.IsDir() {
		return nil
	}

	geminiPath := filepath.Join(rootDir, "GEMINI.md")
	agentsPath := filepath.Join(rootDir, "AGENTS.md")

	targetPath := geminiPath
	if _, err := os.Stat(geminiPath); os.IsNotExist(err) {
		if _, err := os.Stat(agentsPath); err == nil {
			targetPath = agentsPath
		}
	}

	content, err := os.ReadFile(targetPath)
	if err == nil {
		if strings.Contains(string(content), PureBareGuardMarker) {
			return nil
		}
		newContent := strings.TrimRight(string(content), "\n") + "\n\n" + PureBareGuardSnippet
		return os.WriteFile(targetPath, []byte(newContent), 0644)
	}

	if os.IsNotExist(err) {
		initialContent := "# Project Guidelines\n\n" + PureBareGuardSnippet
		return os.WriteFile(geminiPath, []byte(initialContent), 0644)
	}

	return err
}
