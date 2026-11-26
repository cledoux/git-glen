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

	"git-glen/internal/gitcfg"
)

// Scenario represents the detected repository initialization scenario.
type Scenario string

const (
	ScenarioRemoteClone   Scenario = "remote-clone"
	ScenarioCloneConvert  Scenario = "clone-convert"
	ScenarioBareRepair    Scenario = "bare-repair"
	ScenarioLegacyMigrate Scenario = "legacy-migrate"
	ScenarioGreenfield    Scenario = "greenfield"
	ScenarioSubdirInit    Scenario = "subdir-init"
	ScenarioSubdirRepair  Scenario = "subdir-repair"
)

// InitOptions configures optional behavior during repository initialization, conversion, migration, or repair.
type InitOptions struct {
	Layout       Layout
	WorktreesDir string
	VSCode       bool
}

// InitResult contains execution summary metadata from repository initialization.
type InitResult struct {
	Scenario        Scenario
	Layout          Layout
	BaseDir         string
	BareDir         string
	MainDir         string
	WorktreesDir    string
	DefaultBranch   string
	VSCodeWorkspace string
	AgentLinks      []string
	Repaired        bool
	Migrated        bool
	Converted       bool
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
	sb.WriteString("  3. Re-run initialization (or keep your checkout intact with `glen init --layout=subdir`)")
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
	rawTarget := strings.TrimSpace(target)
	implicitCwd := rawTarget == "" || rawTarget == "."
	if implicitCwd {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("failed to get current working directory: %w", err)
		}
		target = cwd
	} else {
		target = rawTarget
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

	// When invoked with no explicit target (or "."), resolve upward if inside an existing
	// workspace checkout so `glen init` / `glen repair` works from subdirectories or worktrees.
	if implicitCwd {
		if repo, ok := gitcfg.Discover(absPath); ok && repo.BaseDir != "" {
			absPath = repo.BaseDir
		}
	}

	// 2. Check if target has .bare/ directory (bare workspace)
	bareDir := filepath.Join(absPath, ".bare")
	if IsBareRepo(bareDir) || isBareDirPresent(bareDir) {
		return ScenarioBareRepair, absPath, nil
	}

	// 3. Check if target has legacy repo/.git layout
	legacyRepoGit := filepath.Join(absPath, "repo", ".git")
	if _, err := os.Stat(legacyRepoGit); err == nil {
		return ScenarioLegacyMigrate, absPath, nil
	}

	// 4. Check if target has .git directory (Standard Clone or subdir workspace)
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
func ProbeDefaultBranch(ctx context.Context, gitDir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	// Try remote symbolic-ref
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+gitDir, "symbolic-ref", "refs/remotes/origin/HEAD")
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
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+gitDir, "symbolic-ref", "HEAD")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		ref := strings.TrimSpace(stdout.String())
		if strings.HasPrefix(ref, "refs/heads/") {
			b := strings.TrimPrefix(ref, "refs/heads/")
			if errVerify := exec.CommandContext(ctx, "git", "--git-dir="+gitDir, "rev-parse", "--verify", "refs/heads/"+b).Run(); errVerify == nil {
				return b, nil
			}
		}
	}

	// Probe origin/main, origin/master, main, or master
	for _, candidate := range []string{"origin/main", "origin/master", "main", "master", "trunk"} {
		if err := exec.CommandContext(ctx, "git", "--git-dir="+gitDir, "rev-parse", "--verify", candidate).Run(); err == nil {
			return strings.TrimPrefix(candidate, "origin/"), nil
		}
	}

	return "main", nil
}

// Init delegates execution to the appropriate initialization workflow based on target detection.
func (m *Manager) Init(ctx context.Context, target string) (*InitResult, error) {
	return m.InitWithOptions(ctx, target, InitOptions{})
}

// InitWithOptions delegates execution to the appropriate initialization workflow and optionally
// creates or updates the workspace .code-workspace file.
func (m *Manager) InitWithOptions(ctx context.Context, target string, opts InitOptions) (*InitResult, error) {
	scen, resolvedPath, err := DetectScenario(target)
	if err != nil {
		return nil, err
	}

	targetLayout, err := resolveTargetLayout(scen, resolvedPath, opts.Layout)
	if err != nil {
		return nil, err
	}

	var res *InitResult
	switch targetLayout {
	case LayoutBare:
		switch scen {
		case ScenarioRemoteClone:
			res, err = m.InitBareRepo(ctx, target, resolvedPath)
		case ScenarioCloneConvert:
			res, err = m.ConvertStandardClone(ctx, resolvedPath)
		case ScenarioBareRepair:
			res, err = m.RepairWorkspace(ctx, resolvedPath)
		case ScenarioLegacyMigrate:
			res, err = m.MigrateLegacySibling(ctx, resolvedPath)
		case ScenarioGreenfield:
			res, err = m.InitGreenfield(ctx, resolvedPath)
		default:
			return nil, fmt.Errorf("unsupported bare initialization scenario: %s", scen)
		}
	case LayoutSubdir:
		switch scen {
		case ScenarioRemoteClone:
			res, err = m.InitSubdirRepo(ctx, target, resolvedPath, opts.WorktreesDir)
		case ScenarioCloneConvert:
			res, err = m.InitOrRepairSubdirClone(ctx, resolvedPath, opts.WorktreesDir)
		case ScenarioGreenfield:
			res, err = m.InitSubdirGreenfield(ctx, resolvedPath, opts.WorktreesDir)
		case ScenarioBareRepair:
			return nil, fmt.Errorf("repository at %s already uses bare layout (.bare/); converting bare to subdir is not supported", resolvedPath)
		case ScenarioLegacyMigrate:
			return nil, fmt.Errorf("legacy sibling layout at %s must be migrated to bare layout (`glen init %s`) or initialized directly inside %s/repo", resolvedPath, resolvedPath, resolvedPath)
		default:
			return nil, fmt.Errorf("unsupported subdir initialization scenario: %s", scen)
		}
	default:
		return nil, fmt.Errorf("unsupported layout: %s", targetLayout)
	}
	if err != nil {
		return nil, err
	}

	gitAdminDir := res.BareDir
	if gitAdminDir == "" {
		gitAdminDir = filepath.Join(res.BaseDir, ".git")
	}
	wsMgr := NewManager(gitAdminDir)
	wsPath, err := wsMgr.SyncVSCodeWorkspace(ctx, opts.VSCode)
	if err != nil {
		return nil, err
	}
	if wsPath != "" {
		res.VSCodeWorkspace = wsPath
	}
	return res, nil
}

func resolveTargetLayout(scen Scenario, resolvedPath string, explicit Layout) (Layout, error) {
	if explicit != "" {
		return ParseLayout(string(explicit))
	}

	switch scen {
	case ScenarioBareRepair, ScenarioLegacyMigrate:
		return LayoutBare, nil
	case ScenarioCloneConvert:
		if repo, ok := gitcfg.Discover(resolvedPath); ok {
			cfg := gitcfg.Load(repo)
			if cfg.State == gitcfg.StateConfigured && cfg.Layout != "" {
				return ParseLayout(cfg.Layout)
			}
		}
	}

	globalCfg := gitcfg.LoadGlobal()
	if globalCfg.DefaultLayout != "" {
		if l, err := ParseLayout(globalCfg.DefaultLayout); err == nil {
			return l, nil
		}
	}
	return LayoutBare, nil
}

// InitBareRepo clones a remote repository into a bare root topology (.bare/ + .git + main/).
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
	_ = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "symbolic-ref", "HEAD", "refs/heads/"+branch).Run()

	mainDir := filepath.Join(absTarget, filepath.FromSlash(branch))

	// 3. Attach primary default-branch worktree
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, "origin/"+branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		cmdRetry := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, branch)
		if outRetry, errRetry := cmdRetry.CombinedOutput(); errRetry != nil {
			return nil, fmt.Errorf("failed to add primary worktree: %w (output: %s / retry: %s)", err, strings.TrimSpace(string(out)), strings.TrimSpace(string(outRetry)))
		}
	}

	// 4. Configure primary worktree
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false").Run()
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "branch", "--set-upstream-to=origin/"+branch, branch).Run()

	agentLinks, err := SyncAgentLinks(absTarget, mainDir)
	if err != nil {
		return nil, err
	}

	return &InitResult{
		Scenario:      ScenarioRemoteClone,
		Layout:        LayoutBare,
		BaseDir:       absTarget,
		BareDir:       bareDir,
		MainDir:       mainDir,
		WorktreesDir:  absTarget,
		DefaultBranch: branch,
		AgentLinks:    agentLinks,
	}, nil
}

// ConvertStandardClone transforms an existing clean standard clone into bare topology.
// Spec: SPEC-0002 (REQ-0002, REQ-0003), SPEC-0001 (REQ-0011, REQ-0012)
func (m *Manager) ConvertStandardClone(ctx context.Context, cloneDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absClone, err := filepath.Abs(cloneDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", cloneDir, err)
	}

	// Guard against wiping active linked worktrees (e.g. inside .worktrees/ which may be ignored).
	existingMgr := NewManager(filepath.Join(absClone, ".git"))
	if wts, wtErr := existingMgr.ListWorktrees(ctx); wtErr == nil && len(wts) > 1 {
		return nil, fmt.Errorf("cannot convert %s to bare layout while %d linked worktree(s) exist; remove linked worktrees first with `glen remove`", absClone, len(wts)-1)
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
	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure bare repository: %w", err)
	}
	_ = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "symbolic-ref", "HEAD", "refs/heads/"+branch).Run()

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

	if err := EnsureRootGitPointer(absClone); err != nil {
		return nil, err
	}

	// 6. Add primary default-branch worktree
	mainDir := filepath.Join(absClone, filepath.FromSlash(branch))
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to add primary worktree: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	// 7. Configure primary worktree core.bare false
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false").Run()

	agentLinks, err := SyncAgentLinks(absClone, mainDir)
	if err != nil {
		return nil, err
	}

	return &InitResult{
		Scenario:      ScenarioCloneConvert,
		Layout:        LayoutBare,
		BaseDir:       absClone,
		BareDir:       bareDir,
		MainDir:       mainDir,
		WorktreesDir:  absClone,
		DefaultBranch: branch,
		AgentLinks:    agentLinks,
		Converted:     true,
	}, nil
}

// RepairWorkspace repairs worktree pointers, enforces configs, and prunes stale locks in a bare repository.
// Spec: SPEC-0002 (REQ-0002), SPEC-0001 (REQ-0013)
func (m *Manager) RepairWorkspace(ctx context.Context, rootDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", rootDir, err)
	}

	bareDir := filepath.Join(absRoot, ".bare")
	if !isBareDirPresent(bareDir) {
		return nil, fmt.Errorf("cannot repair workspace: .bare directory not found in %s", absRoot)
	}

	if err := EnsureRootGitPointer(absRoot); err != nil {
		return nil, err
	}

	branch, _ := ProbeDefaultBranch(ctx, bareDir)
	if branch == "" {
		branch = "main"
	}
	_ = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "symbolic-ref", "HEAD", "refs/heads/"+branch).Run()
	mainDir := filepath.Join(absRoot, filepath.FromSlash(branch))

	// 1. Repair worktrees
	if _, err := os.Stat(mainDir); err == nil {
		_ = exec.CommandContext(ctx, "git", "-C", mainDir, "worktree", "repair").Run()
	}
	cmd := exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "repair")
	cmd.Dir = bareDir
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

	wsPath, _ := bareMgr.SyncVSCodeWorkspace(ctx, false)
	agentLinks, err := SyncAgentLinks(absRoot, mainDir)
	if err != nil {
		return nil, err
	}

	return &InitResult{
		Scenario:        ScenarioBareRepair,
		Layout:          LayoutBare,
		BaseDir:         absRoot,
		BareDir:         bareDir,
		MainDir:         mainDir,
		WorktreesDir:    absRoot,
		DefaultBranch:   branch,
		VSCodeWorkspace: wsPath,
		AgentLinks:      agentLinks,
		Repaired:        true,
	}, nil
}

// MigrateLegacySibling converts a legacy sibling layout (repo/ + worktrees/) into bare topology.
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
	bareMgr := NewManager(bareDir)
	if err := bareMgr.EnsureBareRepoConfig(ctx); err != nil {
		return nil, fmt.Errorf("failed to configure bare repository: %w", err)
	}
	_ = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "symbolic-ref", "HEAD", "refs/heads/"+branch).Run()

	// 5. Remove legacy repo working directory
	_ = os.RemoveAll(repoDir)

	if err := EnsureRootGitPointer(absRoot); err != nil {
		return nil, err
	}

	// 6. Add primary default-branch worktree
	mainDir := filepath.Join(absRoot, filepath.FromSlash(branch))
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to add primary worktree: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	// 7. Configure primary worktree
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false").Run()

	// 8. Defensive prune
	_ = bareMgr.PruneWorktrees(ctx)

	// 9. Remove empty legacy worktrees container if unused
	_ = os.Remove(filepath.Join(absRoot, "worktrees"))

	agentLinks, err := SyncAgentLinks(absRoot, mainDir)
	if err != nil {
		return nil, err
	}

	return &InitResult{
		Scenario:      ScenarioLegacyMigrate,
		Layout:        LayoutBare,
		BaseDir:       absRoot,
		BareDir:       bareDir,
		MainDir:       mainDir,
		WorktreesDir:  absRoot,
		DefaultBranch: branch,
		AgentLinks:    agentLinks,
		Migrated:      true,
	}, nil
}

// InitGreenfield initializes a fresh empty directory into a bare repository.
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
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false").Run()
	ensureFallbackCommitIdentity(ctx, mainDir)

	// 4. Initial commit in main
	readmePath := filepath.Join(mainDir, "README.md")
	_ = os.WriteFile(readmePath, []byte("# Project\n\nInitialized with `glen init`.\n"), 0644)
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "add", "-A").Run()
	_ = exec.CommandContext(ctx, "git", "-C", mainDir, "commit", "-m", "Initial commit").Run()

	return &InitResult{
		Scenario:      ScenarioGreenfield,
		Layout:        LayoutBare,
		BaseDir:       absTarget,
		BareDir:       bareDir,
		MainDir:       mainDir,
		WorktreesDir:  absTarget,
		DefaultBranch: "main",
	}, nil
}

// InitSubdirRepo clones a remote repository as a standard Git clone and configures
// the subdir worktree layout (.git + .worktrees/<branch>).
func (m *Manager) InitSubdirRepo(ctx context.Context, repoURL, targetDir, worktreesDir string) (*InitResult, error) {
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
			if st, errGit := os.Stat(filepath.Join(absTarget, ".git")); errGit == nil && st.IsDir() {
				return m.InitOrRepairSubdirClone(ctx, absTarget, worktreesDir)
			}
			return nil, fmt.Errorf("target directory %s already exists and is not empty", absTarget)
		}
	}

	cmd := exec.CommandContext(ctx, "git", "clone", repoURL, absTarget)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	res, err := m.InitOrRepairSubdirClone(ctx, absTarget, worktreesDir)
	if err != nil {
		return nil, err
	}
	res.Scenario = ScenarioRemoteClone
	res.Repaired = false
	return res, nil
}

// InitOrRepairSubdirClone configures or repairs a standard Git clone in-place for the subdir
// worktree layout without moving working-tree files (safe even on dirty working trees).
func (m *Manager) InitOrRepairSubdirClone(ctx context.Context, cloneDir, worktreesDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absClone, err := filepath.Abs(cloneDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", cloneDir, err)
	}

	gitDir := filepath.Join(absClone, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("cannot initialize subdir layout: %s is not a standard Git directory", gitDir)
	}

	alreadySubdir := false
	if repo, ok := gitcfg.Discover(absClone); ok {
		prev := gitcfg.Load(repo)
		alreadySubdir = prev.State == gitcfg.StateConfigured && prev.Layout == string(LayoutSubdir)
	}

	_ = exec.CommandContext(ctx, "git", "-C", absClone, "worktree", "repair").Run()

	cloneMgr := NewManager(gitDir)
	if err := cloneMgr.EnsureSubdirRepoConfig(ctx, worktreesDir); err != nil {
		return nil, fmt.Errorf("failed to configure subdir repository: %w", err)
	}

	_, cfg := cloneMgr.resolveRepoConfig()
	strat := registeredLayouts[LayoutSubdir]
	wtRoot := strat.worktreesRoot(absClone, cfg.WorktreesDir)
	if err := os.MkdirAll(wtRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktrees directory %s: %w", wtRoot, err)
	}

	if err := cloneMgr.PruneWorktrees(ctx); err != nil {
		return nil, fmt.Errorf("failed to prune worktrees: %w", err)
	}

	branch, _ := ProbeDefaultBranch(ctx, gitDir)
	if branch == "" {
		branch = "main"
	}

	scen := ScenarioSubdirInit
	if alreadySubdir {
		scen = ScenarioSubdirRepair
	}

	wsPath, _ := cloneMgr.SyncVSCodeWorkspace(ctx, false)

	return &InitResult{
		Scenario:        scen,
		Layout:          LayoutSubdir,
		BaseDir:         absClone,
		MainDir:         absClone,
		WorktreesDir:    wtRoot,
		DefaultBranch:   branch,
		VSCodeWorkspace: wsPath,
		Repaired:        alreadySubdir,
	}, nil
}

// InitSubdirGreenfield initializes a fresh standard Git repository configured with the subdir layout.
func (m *Manager) InitSubdirGreenfield(ctx context.Context, targetDir, worktreesDir string) (*InitResult, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path for %s: %w", targetDir, err)
	}
	if err := os.MkdirAll(absTarget, 0755); err != nil {
		return nil, fmt.Errorf("failed to create target directory %s: %w", absTarget, err)
	}

	cmd := exec.CommandContext(ctx, "git", "init", "-b", "main", absTarget)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to git init: %w (%s)", err, out)
	}

	gitDir := filepath.Join(absTarget, ".git")
	cloneMgr := NewManager(gitDir)
	if err := cloneMgr.EnsureSubdirRepoConfig(ctx, worktreesDir); err != nil {
		return nil, fmt.Errorf("failed to configure subdir repository: %w", err)
	}

	_, cfg := cloneMgr.resolveRepoConfig()
	wtRoot := registeredLayouts[LayoutSubdir].worktreesRoot(absTarget, cfg.WorktreesDir)
	if err := os.MkdirAll(wtRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktrees directory %s: %w", wtRoot, err)
	}

	ensureFallbackCommitIdentity(ctx, absTarget)

	readmePath := filepath.Join(absTarget, "README.md")
	_ = os.WriteFile(readmePath, []byte("# Project\n\nInitialized with `glen init --layout=subdir`.\n"), 0644)
	_ = exec.CommandContext(ctx, "git", "-C", absTarget, "add", "README.md").Run()
	_ = exec.CommandContext(ctx, "git", "-C", absTarget, "commit", "-m", "Initial commit").Run()

	return &InitResult{
		Scenario:      ScenarioGreenfield,
		Layout:        LayoutSubdir,
		BaseDir:       absTarget,
		MainDir:       absTarget,
		WorktreesDir:  wtRoot,
		DefaultBranch: "main",
	}, nil
}

func ensureFallbackCommitIdentity(ctx context.Context, worktreeDir string) {
	var out bytes.Buffer
	check := exec.CommandContext(ctx, "git", "-C", worktreeDir, "config", "--get", "user.email")
	check.Stdout = &out
	if err := check.Run(); err == nil && strings.TrimSpace(out.String()) != "" {
		return
	}
	_ = exec.CommandContext(ctx, "git", "-C", worktreeDir, "config", "--worktree", "user.name", "Agent Teams").Run()
	_ = exec.CommandContext(ctx, "git", "-C", worktreeDir, "config", "--worktree", "user.email", "agent@example.com").Run()
}
