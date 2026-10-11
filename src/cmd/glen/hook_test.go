package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupHookWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".bare", "main", "feat/x"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".bare", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	return root
}

func runHookWith(t *testing.T, args []string, payload any) (int, string, string) {
	t.Helper()
	var in []byte
	switch p := payload.(type) {
	case string:
		in = []byte(p)
	default:
		var err error
		if in, err = json.Marshal(p); err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
	}
	var stdout, stderr bytes.Buffer
	code := runHook(args, bytes.NewReader(in), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestHookPreToolUseClaude(t *testing.T) {
	root := setupHookWorkspace(t)
	args := []string{"pre-tool-use", "--agent=claude"}

	t.Run("DeniesEditInMain", func(t *testing.T) {
		code, out, _ := runHookWith(t, args, map[string]any{
			"cwd":        root,
			"tool_name":  "Edit",
			"tool_input": map[string]any{"file_path": filepath.Join(root, "main", "a.go")},
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		var resp struct {
			HookSpecificOutput struct {
				HookEventName            string `json:"hookEventName"`
				PermissionDecision       string `json:"permissionDecision"`
				PermissionDecisionReason string `json:"permissionDecisionReason"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("stdout is not JSON: %v (%q)", err, out)
		}
		h := resp.HookSpecificOutput
		if h.HookEventName != "PreToolUse" || h.PermissionDecision != "deny" || !strings.Contains(h.PermissionDecisionReason, "glen create") {
			t.Errorf("unexpected response: %+v", h)
		}
	})

	t.Run("SilentForFeatureWorktree", func(t *testing.T) {
		code, out, _ := runHookWith(t, args, map[string]any{
			"cwd":        root,
			"tool_name":  "Write",
			"tool_input": map[string]any{"file_path": filepath.Join(root, "feat", "x", "a.go")},
		})
		if code != 0 || out != "" {
			t.Errorf("got code=%d stdout=%q, want 0 and empty", code, out)
		}
	})

	t.Run("DeniesDeleteBranchMerge", func(t *testing.T) {
		_, out, _ := runHookWith(t, args, map[string]any{
			"cwd":        root,
			"tool_name":  "Bash",
			"tool_input": map[string]any{"command": "gh pr merge 3 --squash --delete-branch"},
		})
		if !strings.Contains(out, `"deny"`) {
			t.Errorf("expected deny, got %q", out)
		}
	})

	t.Run("FailsOpenOnGarbage", func(t *testing.T) {
		code, out, errOut := runHookWith(t, args, "not json")
		if code != 0 || out != "" {
			t.Errorf("got code=%d stdout=%q, want 0 and empty", code, out)
		}
		if errOut == "" {
			t.Errorf("expected a diagnostic on stderr")
		}
	})
}

func TestHookPreToolUseAgy(t *testing.T) {
	root := setupHookWorkspace(t)
	args := []string{"pre-tool-use", "--agent=agy"}

	t.Run("DeniesFileArgInMain", func(t *testing.T) {
		code, out, _ := runHookWith(t, args, map[string]any{
			"workspacePaths": []string{root},
			"toolCall": map[string]any{
				"name": "code_action",
				"args": map[string]any{"TargetFile": filepath.Join(root, "main", "a.go"), "Instruction": "x"},
			},
		})
		var resp struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("stdout is not JSON: %v (%q)", err, out)
		}
		if code != 0 || resp.Decision != "deny" || resp.Reason == "" {
			t.Errorf("got code=%d resp=%+v, want deny with reason", code, resp)
		}
	})

	t.Run("RelativeCommandCwd", func(t *testing.T) {
		_, out, _ := runHookWith(t, args, map[string]any{
			"workspacePaths": []string{root},
			"toolCall": map[string]any{
				"name": "run_command",
				"args": map[string]any{"CommandLine": "gh pr merge -d", "Cwd": filepath.Join(root, "feat", "x")},
			},
		})
		if !strings.Contains(out, `"deny"`) {
			t.Errorf("expected deny, got %q", out)
		}
	})

	t.Run("ReadOnlyToolPasses", func(t *testing.T) {
		_, out, _ := runHookWith(t, args, map[string]any{
			"workspacePaths": []string{root},
			"toolCall": map[string]any{
				"name": "view_file",
				"args": map[string]any{"AbsolutePath": filepath.Join(root, "main", "a.go")},
			},
		})
		if strings.TrimSpace(out) != "{}" {
			t.Errorf("got %q, want {}", out)
		}
	})

	t.Run("FailsOpenOnGarbage", func(t *testing.T) {
		code, out, _ := runHookWith(t, args, "not json")
		if code != 0 || strings.TrimSpace(out) != "{}" {
			t.Errorf("got code=%d stdout=%q, want 0 and {}", code, out)
		}
	})
}

func TestHookSessionStartClaude(t *testing.T) {
	root := setupHookWorkspace(t)
	plain := t.TempDir()
	ctxFile := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(ctxFile, []byte("---\ntrigger: model_decision\n---\n\n# Discipline\nUse glen.\n"), 0o644); err != nil {
		t.Fatalf("write context file: %v", err)
	}
	args := []string{"session-start", "--agent=claude", "--context-file", ctxFile}

	t.Run("InjectsInPureBare", func(t *testing.T) {
		_, out, _ := runHookWith(t, args, map[string]any{"cwd": filepath.Join(root, "feat", "x")})
		var resp struct {
			HookSpecificOutput struct {
				HookEventName     string `json:"hookEventName"`
				AdditionalContext string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(out), &resp); err != nil {
			t.Fatalf("stdout is not JSON: %v (%q)", err, out)
		}
		h := resp.HookSpecificOutput
		if h.HookEventName != "SessionStart" || h.AdditionalContext != "# Discipline\nUse glen.\n" {
			t.Errorf("unexpected response: %+v", h)
		}
	})

	t.Run("SilentOutsidePureBare", func(t *testing.T) {
		_, out, _ := runHookWith(t, args, map[string]any{"cwd": plain})
		if out != "" {
			t.Errorf("got %q, want empty", out)
		}
	})
}

func TestHookUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"pre-tool-use"},
		{"pre-tool-use", "--agent=vim"},
		{"--agent=claude"},
	} {
		if code, _, _ := runHookWith(t, args, "{}"); code != 2 {
			t.Errorf("runHook(%v) = %d, want 2", args, code)
		}
	}
}

func TestHookPayloadLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "hook.log")
	t.Setenv(envHookLog, logPath)
	runHookWith(t, []string{"pre-tool-use", "--agent=agy"}, `{"toolCall":{"name":"x"}}`)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), `"toolCall"`) {
		t.Errorf("log missing payload: %q", data)
	}
}
