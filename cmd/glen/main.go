package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git-glen/worktree"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	code := RunGlenCommand(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

// RunGlenCommand executes top-level `glen` (or `git-glen`) subcommands.
func RunGlenCommand(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printGlenUsage(stderr)
		if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
			return 0
		}
		return 2
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "init":
		return runInit(ctx, subArgs, stdout, stderr)
	case "list", "ls", "status":
		return runList(ctx, subArgs, stdout, stderr)
	case "create", "add", "new":
		return runCreate(ctx, subArgs, stdout, stderr)
	case "remove", "rm", "delete", "cleanup":
		return runRemove(ctx, subArgs, stdout, stderr)
	case "prune":
		return runPrune(ctx, subArgs, stdout, stderr)
	case "repair":
		return runInit(ctx, subArgs, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Unknown glen subcommand: %s\n\n", subcommand)
		printGlenUsage(stderr)
		return 2
	}
}

func printGlenUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: glen <subcommand> [options] [arguments]\n")
	fmt.Fprintf(w, "   or: git glen <subcommand> [options] [arguments]\n\n")
	fmt.Fprintf(w, "Guided Git worktree and Pure Bare repository workflows.\n\n")
	fmt.Fprintf(w, "Subcommands:\n")
	fmt.Fprintf(w, "  init [target]                     Initialize, convert, migrate, or repair repository into Pure Bare topology\n")
	fmt.Fprintf(w, "  list                              List active Git worktrees in the workspace\n")
	fmt.Fprintf(w, "  create <branch-or-slug> [base]    Create an isolated worktree at <root>/<branch-name>\n")
	fmt.Fprintf(w, "  remove <branch-or-slug>           Remove a feature worktree, prune locks, and delete its local branch\n")
	fmt.Fprintf(w, "  prune                             Prune stale worktree metadata and locks (--expire now)\n")
	fmt.Fprintf(w, "  repair [target]                   Repair worktree pointers, root .git pointer, and bare config\n\n")
	fmt.Fprintf(w, "Common Options:\n")
	fmt.Fprintf(w, "  -C <dir>                          Change effective directory before resolving workspace\n")
	fmt.Fprintf(w, "  -h, --help                        Show this help message\n")
}

func runInit(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("glen init", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var dir string
	fs.StringVar(&dir, "C", "", "Change directory before executing initialization")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: glen init [options] [target]\n\n")
		fmt.Fprintf(stderr, "Initializes, converts, migrates, or repairs a repository into a Pure Bare topology (.bare/ + .git + main/ + <branch>/).\n\n")
		fmt.Fprintf(stderr, "Arguments:\n")
		fmt.Fprintf(stderr, "  target        Remote Git URL (git@... or https://...), local directory, or omitted for current directory\n\n")
		fmt.Fprintf(stderr, "Options:\n")
		fs.PrintDefaults()
	}

	positionals, err := parseFlagsAndArgs(fs, args)
	if err != nil {
		return 2
	}

	target := ""
	if len(positionals) > 0 {
		target = positionals[0]
	}

	if dir != "" {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			fmt.Fprintf(stderr, "[ERROR] Invalid directory '%s': %v\n", dir, err)
			return 1
		}
		if target == "" || target == "." {
			target = absDir
		} else if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") && !strings.HasPrefix(target, "git@") && !strings.HasPrefix(target, "ssh://") && !filepath.IsAbs(target) {
			target = filepath.Join(absDir, target)
		}
	}

	mgr := worktree.NewManager("")
	res, err := mgr.Init(ctx, target)
	if err != nil {
		var dirtyErr *worktree.DirtyTreeError
		if errors.As(err, &dirtyErr) {
			fmt.Fprintf(stderr, "[ERROR] Working directory contains uncommitted changes or untracked files.\n")
			fmt.Fprintf(stderr, "        glen init will not modify or restructure a dirty repository.\n\n")
			fmt.Fprintf(stderr, "Target directory: %s\n", dirtyErr.Dir)
			fmt.Fprintf(stderr, "Dirty files detected:\n")
			for _, f := range dirtyErr.Files {
				fmt.Fprintf(stderr, "  %s\n", f)
			}
			fmt.Fprintf(stderr, "\nSuggested remediation actions:\n")
			fmt.Fprintf(stderr, "  1. Commit your changes:   git -C %s add -A && git -C %s commit -m \"WIP\"\n", dirtyErr.Dir, dirtyErr.Dir)
			fmt.Fprintf(stderr, "  2. Or stash your changes: git -C %s stash --include-untracked\n", dirtyErr.Dir)
			fmt.Fprintf(stderr, "  3. Re-run initialization: glen init %s\n", dirtyErr.Dir)
			return 1
		}

		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "=== Pure Bare Repository Initialized ===\n")
	fmt.Fprintf(stdout, "Scenario:       %s\n", res.Scenario)
	fmt.Fprintf(stdout, "Base Directory: %s\n", res.BaseDir)
	fmt.Fprintf(stdout, "Bare Database:  %s\n", res.BareDir)
	fmt.Fprintf(stdout, "Root Pointer:   %s/.git -> ./.bare\n", res.BaseDir)
	fmt.Fprintf(stdout, "Main Worktree:  %s\n", res.MainDir)
	fmt.Fprintf(stdout, "Default Branch: %s\n", res.DefaultBranch)
	if res.Converted {
		fmt.Fprintf(stdout, "Note:           Converted standard clone into Pure Bare topology in-place.\n")
	} else if res.Repaired {
		fmt.Fprintf(stdout, "Note:           Repaired worktree pointers, refspecs, and pruned expired locks.\n")
	} else if res.Migrated {
		fmt.Fprintf(stdout, "Note:           Migrated legacy sibling layout into Pure Bare topology.\n")
	}
	fmt.Fprintf(stdout, "\nWorkspace is ready. Run 'glen list' to inspect active worktrees.\n")
	return 0
}

func runList(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("glen list", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var changeDirFlag string
	var jsonOutput bool
	fs.StringVar(&changeDirFlag, "C", "", "Change directory before resolving workspace")
	fs.BoolVar(&jsonOutput, "json", false, "Output worktrees in JSON format")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: glen list [options]\n\n")
		fmt.Fprintf(stderr, "Lists active Git worktrees in the discovered workspace.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	if _, err := parseFlagsAndArgs(fs, args); err != nil {
		return 2
	}

	topo, err := worktree.DiscoverTopology("", changeDirFlag)
	if err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	mgr := worktree.NewManager(topo.RepoDir)
	wts, err := mgr.ListWorktrees(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	if jsonOutput {
		data, err := json.MarshalIndent(wts, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "[ERROR] Failed to format JSON: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}

	fmt.Fprintf(stdout, "=== Glen Workspace: %s ===\n", topo.Name)
	fmt.Fprintf(stdout, "Base Dir:      %s\n", topo.BaseDir)
	fmt.Fprintf(stdout, "Git Database:  %s\n", topo.RepoDir)
	fmt.Fprintf(stdout, "Worktrees Dir: %s\n\n", topo.WorktreesDir)
	fmt.Fprintf(stdout, "%-24s %-12s %s\n", "BRANCH", "COMMIT", "PATH")
	fmt.Fprintf(stdout, "%s\n", strings.Repeat("-", 80))
	for _, wt := range wts {
		branch := wt.Branch
		if wt.Bare {
			branch = "(bare)"
		} else if branch == "" {
			branch = "(detached)"
		}
		shortSHA := wt.Head
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}
		if shortSHA == "" {
			shortSHA = "-"
		}
		fmt.Fprintf(stdout, "%-24s %-12s %s\n", branch, shortSHA, wt.Path)
	}
	return 0
}

func runCreate(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("glen create", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var changeDirFlag, baseBranch, authorName, authorEmail string
	var issueID int
	var fetchRemote bool
	fs.StringVar(&changeDirFlag, "C", "", "Change directory before resolving workspace")
	fs.IntVar(&issueID, "issue", 0, "Optional issue number (formats branch as feat/<issue>-<slug>)")
	fs.StringVar(&baseBranch, "base", "", "Base branch or ref (defaults to origin/main or main)")
	fs.StringVar(&authorName, "name", "", "Per-worktree git user.name")
	fs.StringVar(&authorEmail, "email", "", "Per-worktree git user.email")
	fs.BoolVar(&fetchRemote, "fetch", true, "Fetch origin before creating worktree when remote is configured")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: glen create [options] <branch-or-slug> [base-ref]\n\n")
		fmt.Fprintf(stderr, "Creates an isolated worktree at <root>/<branch-name> with core.bare=false.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	positionals, err := parseFlagsAndArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(positionals) < 1 {
		fmt.Fprintf(stderr, "Error: <branch-or-slug> is required (e.g. glen create feat/my-feature)\n")
		return 2
	}

	branchOrSlug := positionals[0]
	if baseBranch == "" && len(positionals) >= 2 {
		baseBranch = positionals[1]
	}

	topo, err := worktree.DiscoverTopology("", changeDirFlag)
	if err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	mgr := worktree.NewManager(topo.RepoDir)
	if fetchRemote {
		_ = mgr.FetchRemote(ctx, "origin")
	}

	var wtPath, branchName string
	if issueID > 0 {
		wtPath, branchName, err = mgr.CreateFeatureWorktree(ctx, issueID, branchOrSlug, baseBranch, authorName, authorEmail)
	} else {
		wtPath, branchName, err = mgr.CreateWorktree(ctx, branchOrSlug, baseBranch, authorName, authorEmail)
	}
	if err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "=== Worktree Created ===\n")
	fmt.Fprintf(stdout, "Worktree Path: %s\n", wtPath)
	fmt.Fprintf(stdout, "Branch:        %s\n", branchName)
	if authorName != "" || authorEmail != "" {
		fmt.Fprintf(stdout, "Author:        %s <%s>\n", authorName, authorEmail)
	}
	return 0
}

func runRemove(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("glen remove", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var changeDirFlag string
	var keepBranch bool
	fs.StringVar(&changeDirFlag, "C", "", "Change directory before resolving workspace")
	fs.BoolVar(&keepBranch, "keep-branch", false, "Do not delete the local feature branch after removing the worktree")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: glen remove [options] <branch-or-slug>\n\n")
		fmt.Fprintf(stderr, "Removes an isolated worktree at <root>/<branch-name>, prunes locks, and deletes the local branch.\n\nOptions:\n")
		fs.PrintDefaults()
	}

	positionals, err := parseFlagsAndArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(positionals) < 1 {
		fmt.Fprintf(stderr, "Error: <branch-or-slug> is required (e.g. glen remove feat/my-feature)\n")
		return 2
	}

	target := strings.TrimSpace(positionals[0])
	topo, err := worktree.DiscoverTopology("", changeDirFlag)
	if err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	cleanBranch := strings.Trim(strings.ReplaceAll(target, " ", "-"), "/")
	wtPath := filepath.Clean(filepath.Join(topo.WorktreesDir, filepath.FromSlash(cleanBranch)))
	if !topo.IsBare {
		dirSlug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(target, "/", "-"), " ", "-"))
		wtPath = filepath.Clean(filepath.Join(topo.WorktreesDir, dirSlug))
	}
	branchName := target

	mgr := worktree.NewManager(topo.RepoDir)
	if wts, err := mgr.ListWorktrees(ctx); err == nil {
		for _, wt := range wts {
			if wt.Path == wtPath || wt.Branch == target || filepath.Base(wt.Path) == target {
				wtPath = wt.Path
				if wt.Branch != "" {
					branchName = wt.Branch
				}
				break
			}
		}
	}

	branchToDelete := branchName
	if keepBranch {
		branchToDelete = ""
	}

	if err := mgr.RemoveFeatureWorktree(ctx, wtPath, branchToDelete); err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "=== Worktree Removed & Pruned ===\n")
	fmt.Fprintf(stdout, "Removed Path:   %s\n", wtPath)
	if branchToDelete != "" {
		fmt.Fprintf(stdout, "Deleted Branch: %s\n", branchToDelete)
	}
	return 0
}

func runPrune(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("glen prune", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var changeDirFlag string
	fs.StringVar(&changeDirFlag, "C", "", "Change directory before resolving workspace")

	if _, err := parseFlagsAndArgs(fs, args); err != nil {
		return 2
	}

	topo, err := worktree.DiscoverTopology("", changeDirFlag)
	if err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	mgr := worktree.NewManager(topo.RepoDir)
	if err := mgr.PruneWorktrees(ctx); err != nil {
		fmt.Fprintf(stderr, "[ERROR] %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Pruned stale worktree metadata in %s\n", topo.RepoDir)
	return 0
}

// parseFlagsAndArgs allows flags and positional arguments to be interspersed in any order.
func parseFlagsAndArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var flagArgs []string
	var posArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flagArgs = append(flagArgs, arg)
			if !strings.Contains(arg, "=") {
				name := strings.TrimLeft(arg, "-")
				if f := fs.Lookup(name); f != nil {
					if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !bf.IsBoolFlag() {
						if i+1 < len(args) {
							flagArgs = append(flagArgs, args[i+1])
							i++
						}
					}
				}
			}
		} else {
			posArgs = append(posArgs, arg)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	posArgs = append(posArgs, fs.Args()...)
	return posArgs, nil
}
