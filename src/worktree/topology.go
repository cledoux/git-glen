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

const gitConfigTimeout = 5 * time.Second

// Topology describes the discovered Git repository and worktree directory structure.
type Topology struct {
	Name            string `json:"name"`
	Layout          Layout `json:"layout,omitempty"`
	Initialized     bool   `json:"initialized"`
	BaseDir         string `json:"base_dir"`
	RepoURL         string `json:"repo_url,omitempty"`
	BareDir         string `json:"bare_dir,omitempty"`
	RepoDir         string `json:"repo_dir"`
	WorktreesDir    string `json:"worktrees_dir"`
	CurrentWorktree string `json:"current_worktree,omitempty"`
	IsBare          bool   `json:"is_bare"`
	IsWorktree      bool   `json:"is_worktree"`
}

// DiscoverTopology traverses upward from Cwd (or changeDir override) to discover the
// enclosing Git repository topology (bare, subdir, uninitialized standard clone, or legacy sibling).
func DiscoverTopology(cwd, changeDir string) (*Topology, error) {
	effectiveCwd := cwd
	if changeDir != "" {
		if filepath.IsAbs(changeDir) {
			effectiveCwd = changeDir
		} else {
			base := effectiveCwd
			if base == "" {
				var err error
				base, err = os.Getwd()
				if err != nil {
					return nil, fmt.Errorf("failed to get working directory: %w", err)
				}
			}
			effectiveCwd = filepath.Join(base, changeDir)
		}
	}

	if effectiveCwd == "" {
		var err error
		effectiveCwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	effectiveCwd = filepath.Clean(effectiveCwd)
	return discoverTopologyFromDir(effectiveCwd)
}

func discoverTopologyFromDir(startDir string) (*Topology, error) {
	// Check for Legacy Sibling layout first (<root>/repo/.git + <root>/worktrees/)
	if rootDir, ok := findLegacySiblingRoot(startDir); ok {
		return buildLegacySiblingTopology(rootDir, startDir)
	}

	repo, ok := gitcfg.Discover(startDir)
	if !ok {
		return nil, fmt.Errorf("unable to discover Git repository topology from '%s'", startDir)
	}

	cfg := gitcfg.Load(repo)
	if cfg.State == gitcfg.StateConfigured && cfg.Layout != "" {
		if _, err := ParseLayout(cfg.Layout); err != nil {
			return nil, fmt.Errorf("invalid glen.layout in %s/config: %w", repo.CommonDir, err)
		}
	}

	if cfg.Layout == string(LayoutBare) {
		return buildBareTopology(repo.BaseDir, startDir)
	}

	return buildCloneTopology(repo, cfg)
}

func findLegacySiblingRoot(startDir string) (string, bool) {
	curr := startDir
	for {
		legacyRepoPath := filepath.Join(curr, "repo", ".git")
		legacyWTPath := filepath.Join(curr, "worktrees")
		if _, errRepo := os.Stat(legacyRepoPath); errRepo == nil {
			if infoWT, errWT := os.Stat(legacyWTPath); errWT == nil && infoWT.IsDir() {
				// Distinguish legacy sibling from a subdir repo that happens to contain repo/
				if _, errBare := os.Stat(filepath.Join(curr, ".bare")); errBare != nil {
					if _, errGit := os.Stat(filepath.Join(curr, ".git")); errGit != nil {
						return curr, true
					}
				}
			}
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return "", false
		}
		curr = parent
	}
}

func buildBareTopology(rootDir, startDir string) (*Topology, error) {
	bareDir := filepath.Join(rootDir, ".bare")

	currentWT := ""
	isWT := false

	if strings.HasPrefix(startDir, rootDir+string(filepath.Separator)) {
		curr := startDir
		for curr != rootDir && curr != "." && curr != string(filepath.Separator) {
			if info, err := os.Stat(filepath.Join(curr, ".git")); err == nil && !info.IsDir() {
				if rel, errRel := filepath.Rel(rootDir, curr); errRel == nil && rel != "." && rel != "" {
					currentWT = filepath.ToSlash(rel)
					isWT = true
				}
				break
			}
			parent := filepath.Dir(curr)
			if parent == curr {
				break
			}
			curr = parent
		}
	}

	return &Topology{
		Name:            filepath.Base(rootDir),
		Layout:          LayoutBare,
		Initialized:     true,
		BaseDir:         rootDir,
		RepoURL:         probeRemoteOriginURL(bareDir),
		BareDir:         bareDir,
		RepoDir:         bareDir,
		WorktreesDir:    rootDir,
		CurrentWorktree: currentWT,
		IsBare:          true,
		IsWorktree:      isWT,
	}, nil
}

func buildCloneTopology(repo gitcfg.Repo, cfg gitcfg.Config) (*Topology, error) {
	cloneDir := repo.BaseDir
	strat := registeredLayouts[LayoutSubdir]
	wtDir := strat.worktreesRoot(cloneDir, cfg.WorktreesDir)

	var layout Layout
	if cfg.Initialized && cfg.Layout != "" {
		layout = Layout(cfg.Layout)
	}

	currentWT := ""
	isWT := repo.IsLinkedWorktree
	if isWT && repo.CheckoutDir != "" {
		if rel, err := filepath.Rel(wtDir, repo.CheckoutDir); err == nil && rel != "." && rel != "" && !strings.HasPrefix(rel, "..") {
			currentWT = filepath.ToSlash(rel)
		} else {
			currentWT = filepath.Base(repo.CheckoutDir)
		}
	}

	return &Topology{
		Name:            filepath.Base(cloneDir),
		Layout:          layout,
		Initialized:     cfg.Initialized,
		BaseDir:         cloneDir,
		RepoURL:         probeRemoteOriginURL(cloneDir),
		BareDir:         "",
		RepoDir:         repo.CommonDir,
		WorktreesDir:    wtDir,
		CurrentWorktree: currentWT,
		IsBare:          false,
		IsWorktree:      isWT,
	}, nil
}

func buildLegacySiblingTopology(rootDir, startDir string) (*Topology, error) {
	repoDir := filepath.Join(rootDir, "repo")
	wtDir := filepath.Join(rootDir, "worktrees")

	currentWT := ""
	isWT := false

	if strings.HasPrefix(startDir, wtDir+string(filepath.Separator)) {
		rel, err := filepath.Rel(wtDir, startDir)
		if err == nil && rel != "." && rel != "" {
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 0 {
				currentWT = parts[0]
				isWT = true
			}
		}
	}

	return &Topology{
		Name:            filepath.Base(rootDir),
		Layout:          "",
		Initialized:     false,
		BaseDir:         rootDir,
		RepoURL:         probeRemoteOriginURL(repoDir),
		BareDir:         "",
		RepoDir:         repoDir,
		WorktreesDir:    wtDir,
		CurrentWorktree: currentWT,
		IsBare:          false,
		IsWorktree:      isWT,
	}, nil
}

func probeRemoteOriginURL(gitTargetDir string) string {
	if gitTargetDir == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitConfigTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if IsBareRepo(gitTargetDir) {
		cmd = exec.CommandContext(ctx, "git", "--git-dir="+gitTargetDir, "config", "--get", "remote.origin.url")
	} else {
		cmd = exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url")
		cmd.Dir = gitTargetDir
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		return strings.TrimSpace(stdout.String())
	}
	return ""
}
