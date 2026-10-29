package worktree

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// VSCodeFolder represents an entry in the "folders" array of a .code-workspace file.
type VSCodeFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ResolveVSCodeWorkspacePath locates an existing .code-workspace file in rootDir
// (checking <root>/<repo>.code-workspace first, then any <root>/*.code-workspace file).
// If none exists and createIfMissing is true, it returns the default path
// <root>/<repo>.code-workspace with existed=false.
func ResolveVSCodeWorkspacePath(rootDir string, createIfMissing bool) (string, bool, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve root directory %s: %w", rootDir, err)
	}

	defaultName := filepath.Base(absRoot) + ".code-workspace"
	defaultPath := filepath.Join(absRoot, defaultName)
	if info, err := os.Stat(defaultPath); err == nil && !info.IsDir() {
		return defaultPath, true, nil
	}

	matches, err := filepath.Glob(filepath.Join(absRoot, "*.code-workspace"))
	if err == nil && len(matches) > 0 {
		sort.Strings(matches)
		for _, match := range matches {
			if info, statErr := os.Stat(match); statErr == nil && !info.IsDir() {
				return match, true, nil
			}
		}
	}

	if !createIfMissing {
		return "", false, nil
	}
	return defaultPath, false, nil
}

// SyncVSCodeWorkspace synchronizes the "folders" array of the workspace's .code-workspace
// file with the active Git worktrees on disk. If no .code-workspace file exists in <root>
// and createIfMissing is false, it is a no-op and returns ("", nil).
func (m *Manager) SyncVSCodeWorkspace(ctx context.Context, createIfMissing bool) (string, error) {
	if !IsBareRepo(m.RepoDir) {
		return "", nil
	}

	rootDir := filepath.Clean(filepath.Join(m.RepoDir, ".."))
	wsPath, existed, err := ResolveVSCodeWorkspacePath(rootDir, createIfMissing)
	if err != nil || wsPath == "" {
		return "", err
	}

	wts, err := m.ListWorktrees(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees for VS Code workspace sync: %w", err)
	}

	seen := make(map[string]bool)
	var relPaths []string
	for _, wt := range wts {
		if wt.Bare {
			continue
		}
		if wt.Path == "" || wt.Path == m.RepoDir || wt.Path == rootDir {
			continue
		}
		info, statErr := os.Stat(wt.Path)
		if statErr != nil || !info.IsDir() {
			continue
		}
		rel, relErr := filepath.Rel(rootDir, wt.Path)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if rel == "." || rel == "" || rel == ".." || strings.HasPrefix(rel, "../") {
			continue
		}
		if !seen[rel] {
			seen[rel] = true
			relPaths = append(relPaths, rel)
		}
	}

	sort.Slice(relPaths, func(i, j int) bool {
		pI, rankI := isPrimaryWorktreeRel(relPaths[i])
		pJ, rankJ := isPrimaryWorktreeRel(relPaths[j])
		if pI != pJ {
			return pI
		}
		if pI && pJ && rankI != rankJ {
			return rankI < rankJ
		}
		return relPaths[i] < relPaths[j]
	})

	folders := make([]VSCodeFolder, 0, len(relPaths))
	for _, rel := range relPaths {
		folders = append(folders, VSCodeFolder{
			Name: rel,
			Path: rel,
		})
	}

	rawMap := make(map[string]json.RawMessage)
	if existed {
		data, readErr := os.ReadFile(wsPath)
		if readErr != nil {
			return "", fmt.Errorf("failed to read %s: %w", wsPath, readErr)
		}
		if len(bytes.TrimSpace(data)) > 0 {
			if unmarshalErr := json.Unmarshal(data, &rawMap); unmarshalErr != nil {
				return "", fmt.Errorf("failed to parse existing %s: %w", wsPath, unmarshalErr)
			}
		}
	} else {
		rawMap["settings"] = json.RawMessage("{}")
	}

	foldersJSON, err := json.Marshal(folders)
	if err != nil {
		return "", fmt.Errorf("failed to marshal VS Code folders: %w", err)
	}
	rawMap["folders"] = foldersJSON

	outBytes, err := formatVSCodeWorkspaceJSON(rawMap)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(wsPath, outBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", wsPath, err)
	}
	return wsPath, nil
}

func isPrimaryWorktreeRel(rel string) (bool, int) {
	switch rel {
	case "main":
		return true, 0
	case "master":
		return true, 1
	case "trunk":
		return true, 2
	default:
		return false, 3
	}
}

func formatVSCodeWorkspaceJSON(rawMap map[string]json.RawMessage) ([]byte, error) {
	orderedKeys := []string{"folders"}
	if _, hasSettings := rawMap["settings"]; hasSettings {
		orderedKeys = append(orderedKeys, "settings")
	}

	var extraKeys []string
	for k := range rawMap {
		if k == "folders" || k == "settings" {
			continue
		}
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)
	orderedKeys = append(orderedKeys, extraKeys...)

	var buf bytes.Buffer
	buf.WriteString("{\n")
	for idx, k := range orderedKeys {
		var val any
		if err := json.Unmarshal(rawMap[k], &val); err != nil {
			return nil, fmt.Errorf("failed to unmarshal key %q in VS Code workspace: %w", k, err)
		}
		formattedVal, err := json.MarshalIndent(val, "  ", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to format key %q in VS Code workspace: %w", k, err)
		}
		keyJSON, _ := json.Marshal(k)
		buf.WriteString("  ")
		buf.Write(keyJSON)
		buf.WriteString(": ")
		buf.Write(formattedVal)
		if idx < len(orderedKeys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
