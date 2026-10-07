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

const gitConfigTimeout = 5 * time.Second

// Topology describes the discovered Git repository and worktree directory structure.
type Topology struct {
	Name            string `json:"name"`
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
// enclosing Git repository topology (Pure Bare, standard clone, linked worktree, or legacy sibling).
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
	curr := startDir
	for {
		// 1. Check for Pure Bare root (.bare directory)
		barePath := filepath.Join(curr, ".bare")
		if info, err := os.Stat(barePath); err == nil && info.IsDir() {
			return buildPureBareTopology(curr, startDir)
		}

		// 2. Check for Legacy Sibling root (curr contains repo/ + worktrees/)
		legacyRepoPath := filepath.Join(curr, "repo", ".git")
		legacyWTPath := filepath.Join(curr, "worktrees")
		if _, errRepo := os.Stat(legacyRepoPath); errRepo == nil {
			if infoWT, errWT := os.Stat(legacyWTPath); errWT == nil && infoWT.IsDir() {
				return buildLegacySiblingTopology(curr, startDir)
			}
		}

		// 3. Check for .git (directory or worktree pointer file)
		gitPath := filepath.Join(curr, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() {
				parent := filepath.Dir(curr)
				parentWT := filepath.Join(parent, "worktrees")
				if filepath.Base(curr) == "repo" {
					if infoPWT, errPWT := os.Stat(parentWT); errPWT == nil && infoPWT.IsDir() {
						return buildLegacySiblingTopology(parent, startDir)
					}
				}
				return buildStandardCloneTopology(curr)
			}

			return buildWorktreePointerTopology(curr, startDir)
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	return nil, fmt.Errorf("unable to discover Git repository topology from '%s'", startDir)
}

func buildPureBareTopology(rootDir, startDir string) (*Topology, error) {
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

func buildStandardCloneTopology(cloneDir string) (*Topology, error) {
	repoDir := filepath.Join(cloneDir, ".git")
	wtDir := filepath.Join(cloneDir, "worktrees")

	return &Topology{
		Name:         filepath.Base(cloneDir),
		BaseDir:      cloneDir,
		RepoURL:      probeRemoteOriginURL(cloneDir),
		BareDir:      "",
		RepoDir:      repoDir,
		WorktreesDir: wtDir,
		IsBare:       false,
		IsWorktree:   false,
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

func buildWorktreePointerTopology(wtPath, startDir string) (*Topology, error) {
	gitFilePath := filepath.Join(wtPath, ".git")
	data, err := os.ReadFile(gitFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .git pointer file at %s: %w", gitFilePath, err)
	}

	line := strings.TrimSpace(string(data))
	gitdirPrefix := "gitdir: "
	if !strings.HasPrefix(line, gitdirPrefix) {
		return nil, fmt.Errorf("invalid .git worktree pointer format at %s", gitFilePath)
	}

	targetGitDir := strings.TrimSpace(strings.TrimPrefix(line, gitdirPrefix))
	if !filepath.IsAbs(targetGitDir) {
		targetGitDir = filepath.Clean(filepath.Join(wtPath, targetGitDir))
	}

	parent := targetGitDir
	for {
		if filepath.Base(parent) == ".bare" && IsBareRepo(parent) {
			rootDir := filepath.Dir(parent)
			return buildPureBareTopology(rootDir, startDir)
		}
		if filepath.Base(parent) == ".git" {
			repoDir := filepath.Dir(parent)
			grandParent := filepath.Dir(repoDir)
			if filepath.Base(repoDir) == "repo" {
				return buildLegacySiblingTopology(grandParent, startDir)
			}
			return buildStandardCloneTopology(repoDir)
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}

	return &Topology{
		Name:            filepath.Base(wtPath),
		BaseDir:         wtPath,
		RepoURL:         probeRemoteOriginURL(wtPath),
		RepoDir:         targetGitDir,
		WorktreesDir:    filepath.Join(filepath.Dir(wtPath), "worktrees"),
		CurrentWorktree: filepath.Base(wtPath),
		IsBare:          false,
		IsWorktree:      true,
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
