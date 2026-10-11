package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"git-glen/internal/hook"
)

// envHookLog names a file that receives every raw hook payload, for inspecting
// what an agent actually sends (e.g. Antigravity's tool argument names).
const envHookLog = "GLEN_HOOK_LOG"

const (
	agentClaude = "claude"
	agentAgy    = "agy"
)

// runHook adapts agent lifecycle hooks to the guards in internal/hook. It always
// fails open: any parse or I/O error yields a pass-through response and exit 0,
// so a broken glen never blocks an agent's tool calls.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("glen hook", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var agent, contextFile string
	fs.StringVar(&agent, "agent", "", "Agent payload format: claude or agy")
	fs.StringVar(&contextFile, "context-file", "", "session-start: Markdown file injected as context in Pure Bare workspaces")

	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: glen hook <pre-tool-use|session-start> --agent=<claude|agy> [options]\n\n")
		fmt.Fprintf(stderr, "Reads an agent hook payload on stdin and writes the agent's hook response on stdout.\n")
		fmt.Fprintf(stderr, "Fails open: errors never block the agent.\n\n")
		fmt.Fprintf(stderr, "Options:\n")
		fs.PrintDefaults()
	}

	positionals, err := parseFlagsAndArgs(fs, args)
	if err != nil {
		return 2
	}
	if len(positionals) != 1 || (agent != agentClaude && agent != agentAgy) {
		fs.Usage()
		return 2
	}

	payload, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "glen hook: reading stdin: %v\n", err)
		return passThrough()
	}
	logPayload(positionals[0], agent, payload, stderr)

	switch positionals[0] {
	case "pre-tool-use":
		return hookPreToolUse(agent, payload, stdout, stderr)
	case "session-start":
		return hookSessionStart(agent, contextFile, payload, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "glen hook: unknown event %q\n", positionals[0])
		return passThrough()
	}
}

func logPayload(event, agent string, payload []byte, stderr io.Writer) {
	path := os.Getenv(envHookLog)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(stderr, "glen hook: opening %s: %v\n", path, err)
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "# %s %s\n%s\n", agent, event, strings.TrimSpace(string(payload)))
}

// passThrough expresses "no opinion" by writing nothing. Both agents then apply
// their normal permission policy. Antigravity treats any JSON object without an
// explicit "allow" (including {} and {"decision":""}) as a denial, and "allow"
// would bypass its permission prompts, so empty output is the only neutral reply.
func passThrough() int {
	return 0
}

func hookPreToolUse(agent string, payload []byte, stdout, stderr io.Writer) int {
	var action hook.Action
	var err error
	switch agent {
	case agentClaude:
		action, err = parseClaudeToolCall(payload)
	case agentAgy:
		action, err = parseAgyToolCall(payload)
	}
	if err != nil {
		fmt.Fprintf(stderr, "glen hook: parsing %s payload: %v\n", agent, err)
		return passThrough()
	}

	d := hook.Evaluate(action)
	if !d.Deny {
		return passThrough()
	}

	var resp any
	switch agent {
	case agentClaude:
		resp = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": d.Reason,
			},
		}
	case agentAgy:
		resp = map[string]any{"decision": "deny", "reason": d.Reason}
	}
	return writeJSON(resp, stdout, stderr)
}

func hookSessionStart(agent, contextFile string, payload []byte, stdout, stderr io.Writer) int {
	if agent != agentClaude || contextFile == "" {
		return passThrough()
	}
	var in struct {
		Cwd string `json:"cwd"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		fmt.Fprintf(stderr, "glen hook: parsing %s payload: %v\n", agent, err)
		return passThrough()
	}
	if _, ok := hook.FindWorkspace(in.Cwd); !ok {
		return passThrough()
	}
	data, err := os.ReadFile(contextFile)
	if err != nil {
		fmt.Fprintf(stderr, "glen hook: reading context file: %v\n", err)
		return passThrough()
	}
	resp := map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     "SessionStart",
			"additionalContext": stripFrontmatter(string(data)),
		},
	}
	return writeJSON(resp, stdout, stderr)
}

func writeJSON(v any, stdout, stderr io.Writer) int {
	out, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(stderr, "glen hook: encoding response: %v\n", err)
		return passThrough()
	}
	fmt.Fprintln(stdout, string(out))
	return 0
}

// stripFrontmatter removes a leading YAML frontmatter block, which is
// Antigravity rule metadata rather than guidance for the model.
func stripFrontmatter(s string) string {
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		return s
	}
	_, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return s
	}
	return strings.TrimLeft(body, "\n")
}

// parseClaudeToolCall reads Claude Code's PreToolUse payload. File tools carry
// file_path (notebook_path for NotebookEdit); Bash carries command.
func parseClaudeToolCall(payload []byte) (hook.Action, error) {
	var in struct {
		Cwd       string `json:"cwd"`
		ToolName  string `json:"tool_name"`
		ToolInput struct {
			FilePath     string `json:"file_path"`
			NotebookPath string `json:"notebook_path"`
			Command      string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return hook.Action{}, err
	}
	a := hook.Action{Cwd: in.Cwd}
	switch in.ToolName {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		for _, p := range []string{in.ToolInput.FilePath, in.ToolInput.NotebookPath} {
			if p != "" {
				a.Paths = append(a.Paths, p)
			}
		}
	case "Bash":
		a.Command = in.ToolInput.Command
	}
	return a, nil
}

// parseAgyToolCall reads Antigravity's PreToolUse payload. Observed in agy
// 2026-10: write_to_file and replace_file_content carry the target in
// TargetFile; run_command carries CommandLine and Cwd. Other file-mutating tool
// names found in the agy binary are guarded by any string argument whose key
// mentions a file or path. Every other tool passes, so reads never trip the
// edit guard.
func parseAgyToolCall(payload []byte) (hook.Action, error) {
	var in struct {
		WorkspacePaths []string `json:"workspacePaths"`
		ToolCall       struct {
			Name string         `json:"name"`
			Args map[string]any `json:"args"`
		} `json:"toolCall"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return hook.Action{}, err
	}
	a := hook.Action{}
	if len(in.WorkspacePaths) > 0 {
		a.Cwd = in.WorkspacePaths[0]
	}
	if cwd, ok := in.ToolCall.Args["Cwd"].(string); ok && cwd != "" {
		a.Cwd = cwd
	}

	switch in.ToolCall.Name {
	case "run_command":
		a.Command, _ = in.ToolCall.Args["CommandLine"].(string)
	case "write_to_file", "replace_file_content", "multi_replace_file_content",
		"create_file", "edit_file", "delete_file", "edit_notebook":
		for k, v := range in.ToolCall.Args {
			key := strings.ToLower(k)
			if key == "cwd" || !(strings.Contains(key, "file") || strings.Contains(key, "path")) {
				continue
			}
			if s, ok := v.(string); ok && s != "" {
				a.Paths = append(a.Paths, s)
			}
		}
	}
	return a, nil
}
