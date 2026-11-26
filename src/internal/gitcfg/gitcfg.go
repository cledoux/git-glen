// Package gitcfg provides zero-subprocess Git repository discovery and [glen]
// configuration loading for both CLI commands and latency-sensitive agent hooks.
package gitcfg

import (
	"os"
	"path/filepath"
	"strings"
)

// State classifies how a Git repository relates to glen.
type State string

const (
	// StateUninitialized indicates a standard Git repository with no glen.layout
	// configuration and no .bare/ directory.
	StateUninitialized State = "uninitialized"
	// StateImplicitBare indicates a legacy .bare/ workspace without an explicit
	// glen.layout entry in .bare/config.
	StateImplicitBare State = "implicit-bare"
	// StateConfigured indicates an explicit glen.layout setting in $GIT_COMMON_DIR/config.
	StateConfigured State = "configured"
)

const (
	LayoutBare   = "bare"
	LayoutSubdir = "subdir"

	DefaultSubdirWorktreesDir = ".worktrees"
)

// Repo describes the enclosing Git repository and checkout discovered from a
// filesystem path without invoking any git subprocesses.
type Repo struct {
	// BaseDir is the logical workspace root (<root> for bare, <clone> for subdir/standard).
	BaseDir string
	// CommonDir is $GIT_COMMON_DIR (<root>/.bare or <clone>/.git).
	CommonDir string
	// GitDir is the checkout-specific $GIT_DIR (<commonDir>/worktrees/<id>, or CommonDir).
	GitDir string
	// CheckoutDir is the nearest enclosing working-tree root, or empty at a bare root.
	CheckoutDir string
	// IsBareRoot is true when the discovered path is at a .bare workspace root (not inside a linked worktree).
	IsBareRoot bool
	// IsLinkedWorktree is true when CheckoutDir uses a .git pointer file to a worktree metadata dir.
	IsLinkedWorktree bool
}

// Config holds the resolved [glen] settings for a repository (or global defaults).
type Config struct {
	State          State
	Initialized    bool
	Layout         string
	DefaultLayout  string
	WorktreesDir   string
	DefaultBranch  string
	ProtectMain    bool
	AutoIsolate    bool
	ReadOnlyAgents []string
}

type rawSection struct {
	layout         string
	hasLayout      bool
	defaultLayout  string
	worktreesDir   string
	defaultBranch  string
	protectMain    bool
	hasProtectMain bool
	autoIsolate    bool
	hasAutoIsolate bool
	readOnlyAgents []string
}

// Discover walks upward from path (which may be a file or non-existent leaf path)
// to locate the nearest enclosing Git checkout and its shared $GIT_COMMON_DIR.
func Discover(path string) (Repo, bool) {
	if path == "" {
		return Repo{}, false
	}
	curr := filepath.Clean(path)
	if abs, err := filepath.Abs(curr); err == nil {
		curr = abs
	}

	// If curr is an existing regular file or symlink, start from its parent directory;
	// if curr does not exist yet, walk up until an existing directory is reached.
	for {
		info, err := os.Stat(curr)
		if err == nil {
			if !info.IsDir() {
				curr = filepath.Dir(curr)
			}
			break
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return Repo{}, false
		}
		curr = parent
	}

	for {
		gitPath := filepath.Join(curr, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() {
				return Repo{
					BaseDir:     curr,
					CommonDir:   gitPath,
					GitDir:      gitPath,
					CheckoutDir: curr,
				}, true
			}
			if target, ok := readGitdirPointer(gitPath); ok {
				bareCandidate := filepath.Join(curr, ".bare")
				if filepath.Clean(target) == filepath.Clean(bareCandidate) {
					return Repo{
						BaseDir:    curr,
						CommonDir:  bareCandidate,
						GitDir:     bareCandidate,
						IsBareRoot: true,
					}, true
				}
				commonDir := resolveCommonDir(target)
				baseDir := deriveBaseDir(commonDir, curr)
				return Repo{
					BaseDir:          baseDir,
					CommonDir:        commonDir,
					GitDir:           target,
					CheckoutDir:      curr,
					IsLinkedWorktree: true,
				}, true
			}
		}

		barePath := filepath.Join(curr, ".bare")
		if info, err := os.Stat(barePath); err == nil && info.IsDir() {
			return Repo{
				BaseDir:    curr,
				CommonDir:  barePath,
				GitDir:     barePath,
				IsBareRoot: true,
			}, true
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			return Repo{}, false
		}
		curr = parent
	}
}

// Load resolves the effective [glen] configuration for repo by merging global
// Git config defaults with $GIT_COMMON_DIR/config and repository ref metadata.
func Load(repo Repo) Config {
	globalRaw := loadGlobalSection()
	var repoRaw rawSection
	if repo.CommonDir != "" {
		if data, err := os.ReadFile(filepath.Join(repo.CommonDir, "config")); err == nil {
			repoRaw = parseGlenSection(data)
		}
	}

	defaultLayout := LayoutBare
	if globalRaw.defaultLayout != "" {
		defaultLayout = strings.ToLower(globalRaw.defaultLayout)
	}
	if repoRaw.defaultLayout != "" {
		defaultLayout = strings.ToLower(repoRaw.defaultLayout)
	}

	isImplicitBare := repo.IsBareRoot || filepath.Base(repo.CommonDir) == ".bare"

	var state State
	var layout string
	var initialized bool

	switch {
	case repoRaw.hasLayout && repoRaw.layout != "":
		state = StateConfigured
		layout = strings.ToLower(repoRaw.layout)
		initialized = true
	case isImplicitBare:
		state = StateImplicitBare
		layout = LayoutBare
		initialized = true
	default:
		state = StateUninitialized
		layout = ""
		initialized = false
	}

	wtDir := DefaultSubdirWorktreesDir
	if globalRaw.worktreesDir != "" {
		wtDir = globalRaw.worktreesDir
	}
	if repoRaw.worktreesDir != "" {
		wtDir = repoRaw.worktreesDir
	}

	defBranch := globalRaw.defaultBranch
	if repoRaw.defaultBranch != "" {
		defBranch = repoRaw.defaultBranch
	}
	if defBranch == "" {
		defBranch = probeDefaultBranchFast(repo.CommonDir, isImplicitBare || layout == LayoutBare)
	}

	protectMain := initialized
	if globalRaw.hasProtectMain {
		protectMain = globalRaw.protectMain
	}
	if repoRaw.hasProtectMain {
		protectMain = repoRaw.protectMain
	}

	autoIsolate := initialized
	if globalRaw.hasAutoIsolate {
		autoIsolate = globalRaw.autoIsolate
	}
	if repoRaw.hasAutoIsolate {
		autoIsolate = repoRaw.autoIsolate
	}

	var roAgents []string
	roAgents = append(roAgents, globalRaw.readOnlyAgents...)
	roAgents = append(roAgents, repoRaw.readOnlyAgents...)

	return Config{
		State:          state,
		Initialized:    initialized,
		Layout:         layout,
		DefaultLayout:  defaultLayout,
		WorktreesDir:   wtDir,
		DefaultBranch:  defBranch,
		ProtectMain:    protectMain,
		AutoIsolate:    autoIsolate,
		ReadOnlyAgents: roAgents,
	}
}

// LoadGlobal returns global-only [glen] defaults (e.g. defaultLayout when
// running `glen init` outside an existing repository).
func LoadGlobal() Config {
	return Load(Repo{})
}

// ReadHeadBranch reads the symbolic branch name from a gitDir's HEAD file
// (e.g. "main" from "ref: refs/heads/main"). Returns ("", false) when detached or unreadable.
func ReadHeadBranch(gitDir string) (string, bool) {
	if gitDir == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	ref := strings.TrimSpace(string(data))
	if b, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok && b != "" {
		return b, true
	}
	return "", false
}

// EnsureInfoExclude appends pattern to $GIT_COMMON_DIR/info/exclude if not already present.
func EnsureInfoExclude(commonDir, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if commonDir == "" || pattern == "" {
		return nil
	}
	infoDir := filepath.Join(commonDir, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return err
	}
	excludePath := filepath.Join(infoDir, "exclude")
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	var buf strings.Builder
	if len(existing) > 0 {
		buf.Write(existing)
		if existing[len(existing)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}
	buf.WriteString(pattern)
	buf.WriteByte('\n')
	return os.WriteFile(excludePath, []byte(buf.String()), 0o644)
}

func readGitdirPointer(gitFile string) (string, bool) {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	target, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", false
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(gitFile), target)
	}
	return filepath.Clean(target), true
}

func resolveCommonDir(gitDir string) string {
	commondirFile := filepath.Join(gitDir, "commondir")
	if data, err := os.ReadFile(commondirFile); err == nil {
		rel := strings.TrimSpace(string(data))
		if rel != "" {
			if !filepath.IsAbs(rel) {
				rel = filepath.Join(gitDir, rel)
			}
			return filepath.Clean(rel)
		}
	}

	// Fallback for synthetic fixtures lacking a commondir file: walk up for .bare or .git.
	curr := gitDir
	for {
		base := filepath.Base(curr)
		if base == ".bare" || base == ".git" {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return gitDir
		}
		curr = parent
	}
}

func deriveBaseDir(commonDir, fallback string) string {
	base := filepath.Base(commonDir)
	if base == ".bare" || base == ".git" {
		return filepath.Dir(commonDir)
	}
	return fallback
}

func probeDefaultBranchFast(commonDir string, isBare bool) string {
	if commonDir == "" {
		return "main"
	}

	// 1. Check refs/remotes/origin/HEAD symbolic ref file
	if data, err := os.ReadFile(filepath.Join(commonDir, "refs", "remotes", "origin", "HEAD")); err == nil {
		ref := strings.TrimSpace(string(data))
		if b, ok := strings.CutPrefix(ref, "ref: refs/remotes/origin/"); ok && b != "" {
			return b
		}
	}

	packed, _ := os.ReadFile(filepath.Join(commonDir, "packed-refs"))
	packedStr := string(packed)
	refExists := func(branch string) bool {
		if _, err := os.Stat(filepath.Join(commonDir, "refs", "heads", branch)); err == nil {
			return true
		}
		return strings.Contains(packedStr, " refs/heads/"+branch+"\n") ||
			strings.HasSuffix(strings.TrimSpace(packedStr), " refs/heads/"+branch)
	}

	// 2. In bare repositories, .bare/HEAD tracks the default branch unless it points
	// to an unborn ref while real branches exist under refs/heads/.
	if isBare {
		if b, ok := ReadHeadBranch(commonDir); ok {
			headsDir := filepath.Join(commonDir, "refs", "heads")
			if refExists(b) {
				return b
			}
			if entries, err := os.ReadDir(headsDir); (err != nil || len(entries) == 0) && packedStr == "" {
				return b
			}
		}
	}

	// 3. Prefer standard default branch refs when present, falling back to HEAD and "main".
	for _, candidate := range []string{"main", "master", "trunk"} {
		if refExists(candidate) {
			return candidate
		}
	}
	if b, ok := ReadHeadBranch(commonDir); ok {
		return b
	}
	return "main"
}

func loadGlobalSection() rawSection {
	var merged rawSection
	for _, p := range globalConfigPaths() {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			continue
		}
		s := parseGlenSection(data)
		mergeRawSection(&merged, s)
	}
	return merged
}

func globalConfigPaths() []string {
	if custom, ok := os.LookupEnv("GIT_CONFIG_GLOBAL"); ok {
		if custom == "" || custom == os.DevNull {
			return nil
		}
		return []string{custom}
	}
	var paths []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "git", "config"))
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "git", "config"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".gitconfig"))
	}
	return paths
}

func mergeRawSection(dst *rawSection, src rawSection) {
	if src.hasLayout {
		dst.layout = src.layout
		dst.hasLayout = true
	}
	if src.defaultLayout != "" {
		dst.defaultLayout = src.defaultLayout
	}
	if src.worktreesDir != "" {
		dst.worktreesDir = src.worktreesDir
	}
	if src.defaultBranch != "" {
		dst.defaultBranch = src.defaultBranch
	}
	if src.hasProtectMain {
		dst.protectMain = src.protectMain
		dst.hasProtectMain = true
	}
	if src.hasAutoIsolate {
		dst.autoIsolate = src.autoIsolate
		dst.hasAutoIsolate = true
	}
	if len(src.readOnlyAgents) > 0 {
		dst.readOnlyAgents = append(dst.readOnlyAgents, src.readOnlyAgents...)
	}
}

func parseGlenSection(data []byte) rawSection {
	var out rawSection
	inGlen := false

	rawLines := strings.Split(string(data), "\n")
	var logicalLines []string
	var cont strings.Builder

	for _, raw := range rawLines {
		line := strings.TrimRight(raw, "\r")
		stripped := stripCommentOutsideQuotes(line)
		if strings.HasSuffix(stripped, `\`) && !endsWithEscapedBackslash(stripped) {
			cont.WriteString(strings.TrimSuffix(stripped, `\`))
			continue
		}
		if cont.Len() > 0 {
			cont.WriteString(stripped)
			logicalLines = append(logicalLines, cont.String())
			cont.Reset()
		} else {
			logicalLines = append(logicalLines, stripped)
		}
	}
	if cont.Len() > 0 {
		logicalLines = append(logicalLines, cont.String())
	}

	for _, line := range logicalLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			header := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			inGlen = strings.EqualFold(header, "glen")
			continue
		}
		if !inGlen {
			continue
		}

		var key, val string
		hasEq := false
		if eq := strings.IndexByte(trimmed, '='); eq >= 0 {
			key = strings.ToLower(strings.TrimSpace(trimmed[:eq]))
			val = unquoteConfigValue(strings.TrimSpace(trimmed[eq+1:]))
			hasEq = true
		} else {
			key = strings.ToLower(trimmed)
			val = "true"
		}

		switch key {
		case "layout":
			out.layout = strings.TrimSpace(val)
			out.hasLayout = true
		case "defaultlayout":
			out.defaultLayout = strings.TrimSpace(val)
		case "worktreesdir":
			out.worktreesDir = strings.TrimSpace(val)
		case "defaultbranch":
			out.defaultBranch = strings.TrimSpace(val)
		case "protectmain":
			if b, ok := parseGitBool(val, !hasEq); ok {
				out.protectMain = b
				out.hasProtectMain = true
			}
		case "autoisolate":
			if b, ok := parseGitBool(val, !hasEq); ok {
				out.autoIsolate = b
				out.hasAutoIsolate = true
			}
		case "readonlyagents":
			for _, token := range strings.FieldsFunc(val, func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t'
			}) {
				if token != "" {
					out.readOnlyAgents = append(out.readOnlyAgents, token)
				}
			}
		}
	}
	return out
}

func stripCommentOutsideQuotes(s string) string {
	inQuotes := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inQuotes {
			escaped = true
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && (c == '#' || c == ';') {
			return s[:i]
		}
	}
	return s
}

func endsWithEscapedBackslash(s string) bool {
	count := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		count++
	}
	return count > 0 && count%2 == 0
}

func unquoteConfigValue(s string) string {
	var b strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'b':
				b.WriteByte('\b')
			case '\\', '"':
				b.WriteByte(next)
			default:
				b.WriteByte(next)
			}
			i++
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func parseGitBool(val string, bareKey bool) (bool, bool) {
	if bareKey {
		return true, true
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "yes", "on", "1":
		return true, true
	case "false", "no", "off", "0", "":
		return false, true
	default:
		return false, false
	}
}
