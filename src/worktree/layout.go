package worktree

import (
	"fmt"
	"path/filepath"
	"strings"

	"git-glen/internal/gitcfg"
)

// Layout identifies how a repository organizes its primary checkout and linked worktrees.
type Layout string

const (
	// LayoutBare stores the bare Git database at <root>/.bare with all branches
	// (including the default branch) checked out as peer worktrees at <root>/<branch>.
	LayoutBare Layout = gitcfg.LayoutBare

	// LayoutSubdir keeps a standard Git clone in-place at <repo>/.git and stores
	// linked feature worktrees inside <repo>/.worktrees/<branch> (or glen.worktreesDir).
	LayoutSubdir Layout = gitcfg.LayoutSubdir

	// DefaultSubdirWorktreesDir is the default hidden subdirectory for LayoutSubdir worktrees.
	DefaultSubdirWorktreesDir = gitcfg.DefaultSubdirWorktreesDir
)

// ParseLayout validates and normalizes a layout name.
func ParseLayout(raw string) (Layout, error) {
	norm := Layout(strings.ToLower(strings.TrimSpace(raw)))
	if _, ok := registeredLayouts[norm]; ok {
		return norm, nil
	}
	return "", fmt.Errorf("unsupported layout %q: valid layouts are %q and %q", raw, LayoutBare, LayoutSubdir)
}

// layoutStrategy abstracts path calculation and repository exclusion conventions
// so additional layouts (such as sibling or external) can be registered cleanly.
type layoutStrategy interface {
	name() Layout
	worktreesRoot(baseDir, configuredWorktreesDir string) string
	worktreePath(worktreesRoot, branchName string) string
	excludePattern(baseDir, worktreesRoot string) string
}

var registeredLayouts = map[Layout]layoutStrategy{
	LayoutBare:   bareLayoutStrategy{},
	LayoutSubdir: subdirLayoutStrategy{},
}

func strategyFor(l Layout) (layoutStrategy, error) {
	s, ok := registeredLayouts[l]
	if !ok {
		return nil, fmt.Errorf("unsupported layout %q: valid layouts are %q and %q", l, LayoutBare, LayoutSubdir)
	}
	return s, nil
}

func cleanBranchRelPath(branchName string) string {
	clean := strings.Trim(strings.ReplaceAll(strings.TrimSpace(branchName), " ", "-"), "/")
	return filepath.FromSlash(clean)
}

type bareLayoutStrategy struct{}

func (bareLayoutStrategy) name() Layout { return LayoutBare }

func (bareLayoutStrategy) worktreesRoot(baseDir, _ string) string {
	return filepath.Clean(baseDir)
}

func (bareLayoutStrategy) worktreePath(worktreesRoot, branchName string) string {
	return filepath.Clean(filepath.Join(worktreesRoot, cleanBranchRelPath(branchName)))
}

func (bareLayoutStrategy) excludePattern(_, _ string) string {
	return ""
}

type subdirLayoutStrategy struct{}

func (subdirLayoutStrategy) name() Layout { return LayoutSubdir }

func (subdirLayoutStrategy) worktreesRoot(baseDir, configuredWorktreesDir string) string {
	wtDir := strings.TrimSpace(configuredWorktreesDir)
	if wtDir == "" {
		wtDir = DefaultSubdirWorktreesDir
	}
	if filepath.IsAbs(wtDir) {
		return filepath.Clean(wtDir)
	}
	return filepath.Clean(filepath.Join(baseDir, wtDir))
}

func (subdirLayoutStrategy) worktreePath(worktreesRoot, branchName string) string {
	return filepath.Clean(filepath.Join(worktreesRoot, cleanBranchRelPath(branchName)))
}

func (subdirLayoutStrategy) excludePattern(baseDir, worktreesRoot string) string {
	rel, err := filepath.Rel(filepath.Clean(baseDir), filepath.Clean(worktreesRoot))
	if err != nil || rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return "/" + strings.Trim(filepath.ToSlash(rel), "/") + "/"
}
