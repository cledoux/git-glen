package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentLinkCandidates lists the top-level agent configuration directories and
// instruction files that SyncAgentLinks exposes at a Pure Bare workspace root
// via relative symlinks into the primary worktree (e.g., .agents -> main/.agents).
var AgentLinkCandidates = []string{
	".agents",
	".claude",
	"AGENTS.md",
	"GEMINI.md",
	"CLAUDE.md",
}

// SyncAgentLinks synchronizes relative symlinks at baseDir for any top-level
// agent configuration entries (see AgentLinkCandidates) present in mainDir.
//
// Existing non-symlink files or directories at baseDir are never overwritten,
// and valid user-managed symlinks pointing elsewhere are left untouched.
// Dangling symlinks or absolute symlinks pointing to the same target in mainDir
// are replaced with clean relative symlinks. Stale symlinks previously pointing
// into mainDir whose targets no longer exist are removed.
//
// Returns the names of entries currently symlinked into mainDir.
func SyncAgentLinks(baseDir, mainDir string) ([]string, error) {
	if baseDir == "" || mainDir == "" {
		return nil, nil
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base directory %s: %w", baseDir, err)
	}
	absMain, err := filepath.Abs(mainDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve main worktree directory %s: %w", mainDir, err)
	}

	mainInfo, err := os.Stat(absMain)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to inspect main worktree %s: %w", absMain, err)
	}
	if !mainInfo.IsDir() {
		return nil, nil
	}

	relMain, err := filepath.Rel(absBase, absMain)
	if err != nil || relMain == "." || relMain == ".." || strings.HasPrefix(relMain, ".."+string(filepath.Separator)) {
		return nil, nil
	}

	var linked []string
	for _, name := range AgentLinkCandidates {
		srcPath := filepath.Join(absMain, name)
		dstPath := filepath.Join(absBase, name)
		wantTarget := filepath.Join(relMain, name)

		_, srcErr := os.Lstat(srcPath)
		if srcErr != nil && !os.IsNotExist(srcErr) {
			return nil, fmt.Errorf("failed to inspect %s: %w", srcPath, srcErr)
		}

		dstInfo, dstErr := os.Lstat(dstPath)
		if dstErr != nil && !os.IsNotExist(dstErr) {
			return nil, fmt.Errorf("failed to inspect %s: %w", dstPath, dstErr)
		}

		if os.IsNotExist(srcErr) {
			// Target does not exist in mainDir; prune stale symlink if glen created it.
			if dstErr == nil && dstInfo.Mode()&os.ModeSymlink != 0 {
				if curTarget, readErr := os.Readlink(dstPath); readErr == nil && isMainTarget(curTarget, wantTarget, srcPath) {
					if err := os.Remove(dstPath); err != nil {
						return nil, fmt.Errorf("failed to remove stale agent symlink %s: %w", dstPath, err)
					}
				}
			}
			continue
		}

		// Target exists in mainDir.
		if os.IsNotExist(dstErr) {
			if err := os.Symlink(wantTarget, dstPath); err != nil {
				return nil, fmt.Errorf("failed to create agent symlink %s -> %s: %w", dstPath, wantTarget, err)
			}
			linked = append(linked, name)
			continue
		}

		// Never clobber a real file or directory at workspace root.
		if dstInfo.Mode()&os.ModeSymlink == 0 {
			continue
		}

		curTarget, err := os.Readlink(dstPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read symlink %s: %w", dstPath, err)
		}
		if curTarget == wantTarget {
			linked = append(linked, name)
			continue
		}

		// Repair broken symlinks or normalize absolute symlinks pointing to srcPath,
		// while preserving valid user-managed symlinks pointing elsewhere.
		_, statErr := os.Stat(dstPath)
		if os.IsNotExist(statErr) || isMainTarget(curTarget, wantTarget, srcPath) {
			if err := os.Remove(dstPath); err != nil {
				return nil, fmt.Errorf("failed to replace symlink %s: %w", dstPath, err)
			}
			if err := os.Symlink(wantTarget, dstPath); err != nil {
				return nil, fmt.Errorf("failed to create agent symlink %s -> %s: %w", dstPath, wantTarget, err)
			}
			linked = append(linked, name)
		}
	}

	return linked, nil
}

func isMainTarget(curTarget, wantTarget, absSrcPath string) bool {
	if filepath.Clean(curTarget) == filepath.Clean(wantTarget) {
		return true
	}
	return filepath.IsAbs(curTarget) && filepath.Clean(curTarget) == filepath.Clean(absSrcPath)
}
