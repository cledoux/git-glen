// Package hook implements the agent-neutral repository guardrails that back the
// `glen hook` subcommand. Agent-specific payload parsing and response encoding
// live in the CLI; this package only decides whether an action is allowed or
// has tool arguments rewritten.
package hook

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"git-glen/internal/gitcfg"
)

// EnvAllowDefaultBranchEdits disables the default-branch edit guard when set to
// a non-empty value, for deliberate maintenance work directly on the default branch.
const EnvAllowDefaultBranchEdits = "GLEN_ALLOW_MAIN_EDITS"

// EnvAutoIsolate overrides automatic worktree isolation for spawned parallel
// agents when set to a boolean value ("1"/"true" or "0"/"false").
const EnvAutoIsolate = "GLEN_AUTO_ISOLATE"

// SpawnAction describes an agent or worker spawn tool call from a coordinator.
type SpawnAction struct {
	// AgentIdent is the subagent type name or swarm worker roster name.
	AgentIdent string
	// Workspace is the requested workspace mode (e.g. "", "inherit", "share", "branch").
	Workspace string
	// WorkspaceKey is the exact tool argument key to overwrite (e.g. "Workspace" or "workspace").
	WorkspaceKey string
}

// Action is a normalized tool call from any supported agent.
type Action struct {
	// Cwd resolves relative Paths and repository configuration.
	Cwd string
	// Paths are files the tool will create or modify.
	Paths []string
	// Command is the shell command line, if the tool runs one.
	Command string
	// Spawn is populated when the tool spawns a subagent or swarm worker.
	Spawn *SpawnAction
}

// Decision is the guard verdict. A zero Decision allows the action untouched.
type Decision struct {
	Deny      bool
	Reason    string
	Overwrite map[string]any
}

var defaultReadOnlyAgentPatterns = []string{
	"explore",
	"plan",
	"narrator",
	"monitor",
	"image-generator",
	"doc_reviewer",
	"doc-reviewer",
	"research*",
	"*researcher",
	"*reviewer",
	"*auditor",
	"*verifier",
	"verifier*",
	"*guide",
}

// Evaluate applies every guard to the action and returns a denial or argument overwrite.
func Evaluate(a Action) Decision {
	if os.Getenv(EnvAllowDefaultBranchEdits) == "" {
		for _, p := range a.Paths {
			if d := checkDefaultBranchEdit(a.Cwd, p); d.Deny {
				return d
			}
		}
	}
	if a.Command != "" {
		if d := checkDeleteBranchMerge(a.Command); d.Deny {
			return d
		}
	}
	if a.Spawn != nil {
		if d := checkSpawnAutoIsolate(a.Cwd, *a.Spawn); len(d.Overwrite) > 0 {
			return d
		}
	}
	return Decision{}
}

// Workspace describes an initialized glen workspace (bare or subdir) and its default branch.
type Workspace struct {
	Root          string
	Layout        string
	DefaultBranch string
	ProtectMain   bool
}

// FindWorkspace locates the enclosing glen-initialized workspace (bare or subdir)
// for dir without invoking any git subprocesses. Uninitialized standard clones
// return (_, false).
func FindWorkspace(dir string) (Workspace, bool) {
	repo, ok := gitcfg.Discover(dir)
	if !ok {
		return Workspace{}, false
	}
	cfg := gitcfg.Load(repo)
	if !cfg.Initialized {
		return Workspace{}, false
	}
	if cfg.Layout != gitcfg.LayoutBare && cfg.Layout != gitcfg.LayoutSubdir {
		return Workspace{}, false
	}
	return Workspace{
		Root:          repo.BaseDir,
		Layout:        cfg.Layout,
		DefaultBranch: cfg.DefaultBranch,
		ProtectMain:   cfg.ProtectMain,
	}, true
}

func checkSpawnAutoIsolate(cwd string, sp SpawnAction) Decision {
	if cwd == "" || sp.WorkspaceKey == "" {
		return Decision{}
	}
	ws := strings.TrimSpace(sp.Workspace)
	if ws != "" && !strings.EqualFold(ws, "inherit") {
		return Decision{}
	}

	repo, ok := gitcfg.Discover(cwd)
	if !ok {
		return Decision{}
	}
	cfg := gitcfg.Load(repo)

	enabled := cfg.AutoIsolate
	if rawEnv, hasEnv := os.LookupEnv(EnvAutoIsolate); hasEnv && strings.TrimSpace(rawEnv) != "" {
		switch strings.ToLower(strings.TrimSpace(rawEnv)) {
		case "1", "true", "yes", "on":
			enabled = true
		case "0", "false", "no", "off":
			enabled = false
		}
	}
	if !enabled {
		return Decision{}
	}

	if isReadOnlyAgent(sp.AgentIdent, cfg.ReadOnlyAgents) {
		return Decision{}
	}

	return Decision{
		Overwrite: map[string]any{
			sp.WorkspaceKey: "share",
		},
	}
}

func isReadOnlyAgent(ident string, extraPatterns []string) bool {
	norm := strings.ToLower(strings.TrimSpace(ident))
	if norm == "" {
		return false
	}
	for _, pat := range defaultReadOnlyAgentPatterns {
		if matchAgentPattern(pat, norm) {
			return true
		}
	}
	for _, pat := range extraPatterns {
		if matchAgentPattern(strings.ToLower(strings.TrimSpace(pat)), norm) {
			return true
		}
	}
	return false
}

func matchAgentPattern(pattern, ident string) bool {
	if pattern == "" {
		return false
	}
	if pattern == ident {
		return true
	}
	matched, err := path.Match(pattern, ident)
	return err == nil && matched
}

func checkDefaultBranchEdit(cwd, path string) Decision {
	if path == "" {
		return Decision{}
	}
	if !filepath.IsAbs(path) {
		if cwd == "" {
			return Decision{}
		}
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)

	resolved := resolveSymlinkPrefix(path)
	repo, ok := gitcfg.Discover(resolved)
	if !ok {
		repo, ok = gitcfg.Discover(path)
		if !ok {
			return Decision{}
		}
	}

	cfg := gitcfg.Load(repo)
	if !cfg.Initialized || !cfg.ProtectMain {
		return Decision{}
	}

	switch cfg.Layout {
	case gitcfg.LayoutBare:
		protected := filepath.Join(repo.BaseDir, filepath.FromSlash(cfg.DefaultBranch))
		protectedResolved := resolveSymlinkPrefix(protected)
		inDefaultPath := isWithin(path, protected) || isWithin(resolved, protectedResolved)
		inDefaultLinked := repo.IsLinkedWorktree && isHeadBranch(repo.GitDir, cfg.DefaultBranch, false)
		if !inDefaultPath && !inDefaultLinked {
			return Decision{}
		}
		return Decision{
			Deny: true,
			Reason: "glen: refusing to edit " + path + " inside the default-branch worktree (" + protected + "). " +
				"Create an isolated worktree with `glen create <branch-name>` (or the `worktree` skill) and make the change there. " +
				"Set " + EnvAllowDefaultBranchEdits + "=1 to override deliberately.",
		}

	case gitcfg.LayoutSubdir:
		if repo.IsLinkedWorktree {
			if !isHeadBranch(repo.GitDir, cfg.DefaultBranch, false) {
				return Decision{}
			}
		} else {
			if !isHeadBranch(repo.GitDir, cfg.DefaultBranch, true) {
				return Decision{}
			}
		}
		return Decision{
			Deny: true,
			Reason: "glen: refusing to edit " + path + " on the default branch (" + cfg.DefaultBranch + ") in " + repo.BaseDir + ". " +
				"Create an isolated worktree with `glen create <branch-name>` (or the `worktree` skill) and make the change there. " +
				"Set " + EnvAllowDefaultBranchEdits + "=1 to override deliberately.",
		}
	}

	return Decision{}
}

func isHeadBranch(gitDir, wantBranch string, defaultTrueOnMissing bool) bool {
	b, ok := gitcfg.ReadHeadBranch(gitDir)
	if ok {
		return b == wantBranch
	}
	if !defaultTrueOnMissing {
		return false
	}
	_, err := os.Stat(filepath.Join(gitDir, "HEAD"))
	return os.IsNotExist(err)
}

func isWithin(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// resolveSymlinkPrefix resolves symlinks along the deepest existing prefix of
// path so edits targeting a workspace-root symlink into main/ (such as
// <root>/.agents/...) resolve to their real target even when the leaf file does
// not yet exist.
func resolveSymlinkPrefix(path string) string {
	curr := filepath.Clean(path)
	var tail []string
	for hops := 0; hops < 16; {
		if resolved, err := filepath.EvalSymlinks(curr); err == nil {
			parts := make([]string, 0, 1+len(tail))
			parts = append(parts, resolved)
			for i := len(tail) - 1; i >= 0; i-- {
				parts = append(parts, tail[i])
			}
			return filepath.Clean(filepath.Join(parts...))
		}
		if info, err := os.Lstat(curr); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(curr); err == nil {
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(curr), target)
				}
				curr = filepath.Clean(target)
				hops++
				continue
			}
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return filepath.Clean(path)
		}
		tail = append(tail, filepath.Base(curr))
		curr = parent
	}
	return filepath.Clean(path)
}

// checkDeleteBranchMerge blocks `gh pr merge --delete-branch`, which makes gh
// run `git checkout <default>` locally and collides when the default branch is
// checked out in another worktree.
func checkDeleteBranchMerge(command string) Decision {
	for _, seg := range splitShellSegments(command) {
		fields := strings.Fields(seg)
		if !hasSubsequence(fields, "gh", "pr", "merge") {
			continue
		}
		for _, f := range fields {
			if f == "-d" || f == "--delete-branch" || strings.HasPrefix(f, "--delete-branch=") {
				return Decision{
					Deny: true,
					Reason: "glen: `gh pr merge --delete-branch` checks out the default branch locally, which collides when the default branch is checked out in another worktree. " +
						"Merge without --delete-branch (enable `gh repo edit --delete-branch-on-merge` for server-side cleanup), then run `glen remove <branch-name>`.",
				}
			}
		}
	}
	return Decision{}
}

func splitShellSegments(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool {
		return r == ';' || r == '&' || r == '|' || r == '\n'
	})
}

// hasSubsequence reports whether want appears in fields as consecutive tokens.
func hasSubsequence(fields []string, want ...string) bool {
	for i := 0; i+len(want) <= len(fields); i++ {
		match := true
		for j, w := range want {
			if fields[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
