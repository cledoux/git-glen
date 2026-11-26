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
	isBare := IsBareRepo(m.RepoDir)
	repo, cfg := m.resolveRepoConfig()

	var rootDir, wtRoot string
	switch {
	case isBare || cfg.Layout == string(LayoutBare):
		rootDir = filepath.Clean(filepath.Join(m.RepoDir, ".."))
		wtRoot = rootDir
	case cfg.Layout == string(LayoutSubdir) && repo.BaseDir != "":
		rootDir = filepath.Clean(repo.BaseDir)
		wtRoot = registeredLayouts[LayoutSubdir].worktreesRoot(rootDir, cfg.WorktreesDir)
	default:
		return "", nil
	}

	wsPath, existed, err := ResolveVSCodeWorkspacePath(rootDir, createIfMissing)
	if err != nil || wsPath == "" {
		return "", err
	}

	wts, err := m.ListWorktrees(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list worktrees for VS Code workspace sync: %w", err)
	}

	seen := make(map[string]bool)
	var folders []VSCodeFolder
	for _, wt := range wts {
		if wt.Bare || wt.Path == "" || wt.Path == m.RepoDir {
			continue
		}
		wtClean := filepath.Clean(wt.Path)
		info, statErr := os.Stat(wtClean)
		if statErr != nil || !info.IsDir() {
			continue
		}

		if wtClean == rootDir {
			if !isBare && !seen["."] {
				seen["."] = true
				folders = append(folders, VSCodeFolder{
					Name: filepath.Base(rootDir),
					Path: ".",
				})
			}
			continue
		}

		relPath, relErr := filepath.Rel(rootDir, wtClean)
		if relErr != nil {
			continue
		}
		relPath = filepath.ToSlash(filepath.Clean(relPath))
		if relPath == "." || relPath == "" || relPath == ".." || strings.HasPrefix(relPath, "../") {
			continue
		}
		if seen[relPath] {
			continue
		}
		seen[relPath] = true

		displayName := relPath
		if wtRoot != rootDir {
			if wtRel, wtErr := filepath.Rel(wtRoot, wtClean); wtErr == nil {
				wtRelSlash := filepath.ToSlash(filepath.Clean(wtRel))
				if wtRelSlash != "." && wtRelSlash != "" && !strings.HasPrefix(wtRelSlash, "../") {
					displayName = wtRelSlash
				}
			}
		}
		folders = append(folders, VSCodeFolder{
			Name: displayName,
			Path: relPath,
		})
	}

	sort.Slice(folders, func(i, j int) bool {
		pI, rankI := isPrimaryWorktreeFolder(folders[i])
		pJ, rankJ := isPrimaryWorktreeFolder(folders[j])
		if pI != pJ {
			return pI
		}
		if pI && pJ && rankI != rankJ {
			return rankI < rankJ
		}
		return folders[i].Name < folders[j].Name
	})

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

	if folders == nil {
		folders = []VSCodeFolder{}
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

func isPrimaryWorktreeFolder(f VSCodeFolder) (bool, int) {
	if f.Path == "." {
		return true, 0
	}
	switch f.Path {
	case "main":
		return true, 1
	case "master":
		return true, 2
	case "trunk":
		return true, 3
	default:
		return false, 4
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
