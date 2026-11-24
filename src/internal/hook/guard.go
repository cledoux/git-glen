// Package hook implements the agent-neutral Pure Bare guardrails that back the
// `glen hook` subcommand. Agent-specific payload parsing and response encoding
// live in the CLI; this package only decides whether an action is allowed.
package hook

import (
	"os"
	"path/filepath"
	"strings"
)

// EnvAllowDefaultBranchEdits disables the default-branch edit guard when set to
// a non-empty value, for deliberate maintenance work directly in <root>/main.
const EnvAllowDefaultBranchEdits = "GLEN_ALLOW_MAIN_EDITS"

// Action is a normalized tool call from any supported agent.
type Action struct {
	// Cwd resolves relative Paths.
	Cwd string
	// Paths are files the tool will create or modify.
	Paths []string
	// Command is the shell command line, if the tool runs one.
	Command string
}

// Decision is the guard verdict. A zero Decision allows the action.
type Decision struct {
	Deny   bool
	Reason string
}

// Evaluate applies every guard to the action and returns the first denial.
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
	return Decision{}
}

// Workspace is a Pure Bare root and the worktree directory of its default branch.
type Workspace struct {
	Root          string
	DefaultBranch string
}

// FindWorkspace walks up from dir to the nearest Pure Bare root (a directory
// containing .bare/). It deliberately avoids worktree.DiscoverTopology, which
// shells out to git, because hooks run on every tool call.
func FindWorkspace(dir string) (Workspace, bool) {
	curr := filepath.Clean(dir)
	for {
		bare := filepath.Join(curr, ".bare")
		if info, err := os.Stat(bare); err == nil && info.IsDir() {
			return Workspace{Root: curr, DefaultBranch: defaultBranch(bare)}, true
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return Workspace{}, false
		}
		curr = parent
	}
}

// defaultBranch reads the bare repository's HEAD, which glen init points at the
// remote's default branch. It falls back to "main" when HEAD is unreadable.
func defaultBranch(bareDir string) string {
	data, err := os.ReadFile(filepath.Join(bareDir, "HEAD"))
	if err != nil {
		return "main"
	}
	ref := strings.TrimSpace(string(data))
	if b, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok && b != "" {
		return b
	}
	return "main"
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
	ws, ok := FindWorkspace(filepath.Dir(resolved))
	if !ok {
		ws, ok = FindWorkspace(filepath.Dir(path))
		if !ok {
			return Decision{}
		}
	}
	protected := filepath.Join(ws.Root, filepath.FromSlash(ws.DefaultBranch))
	protectedResolved := resolveSymlinkPrefix(protected)
	if !isWithin(path, protected) && !isWithin(resolved, protectedResolved) {
		return Decision{}
	}
	return Decision{
		Deny: true,
		Reason: "glen: refusing to edit " + path + " inside the default-branch worktree (" + protected + "). " +
			"Create an isolated worktree with `glen create <branch-name>` (or the `worktree` skill) and make the change there. " +
			"Set " + EnvAllowDefaultBranchEdits + "=1 to override deliberately.",
	}
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
// run `git checkout <default>` locally and collides with the <root>/main worktree.
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
					Reason: "glen: `gh pr merge --delete-branch` checks out the default branch locally, which collides with the <root>/main worktree. " +
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
