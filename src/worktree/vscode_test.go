package worktree

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"git-glen/internal/testutil"
)

type parsedVSCodeWorkspace struct {
	Folders    []VSCodeFolder `json:"folders"`
	Settings   map[string]any `json:"settings"`
	Extensions map[string]any `json:"extensions"`
}

func readVSCodeWorkspaceFile(t *testing.T, path string) parsedVSCodeWorkspace {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read .code-workspace at %s: %v", path, err)
	}
	var ws parsedVSCodeWorkspace
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("failed to unmarshal .code-workspace JSON: %v\nraw:\n%s", err, string(data))
	}
	return ws
}

func TestVSCodeWorkspace_DefaultOff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	emptyDir := testutil.SetupGreenfieldDirectory(t)
	mgr := NewManager("")

	res, err := mgr.Init(ctx, emptyDir)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if res.VSCodeWorkspace != "" {
		t.Errorf("expected empty VSCodeWorkspace when --vscode not set, got %q", res.VSCodeWorkspace)
	}

	wsFile := filepath.Join(emptyDir, filepath.Base(emptyDir)+".code-workspace")
	if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
		t.Errorf("expected no .code-workspace file on default init, got err=%v", err)
	}

	bareMgr := NewManager(res.BareDir)
	wtDir, branch, err := bareMgr.CreateWorktree(ctx, "feat/no-vscode", "main", "", "")
	if err != nil {
		t.Fatalf("CreateWorktree failed: %v", err)
	}
	if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
		t.Errorf("expected no .code-workspace file after CreateWorktree, got err=%v", err)
	}

	if err := bareMgr.RemoveFeatureWorktree(ctx, wtDir, branch); err != nil {
		t.Fatalf("RemoveFeatureWorktree failed: %v", err)
	}
	if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
		t.Errorf("expected no .code-workspace file after RemoveFeatureWorktree, got err=%v", err)
	}

	if err := bareMgr.PruneWorktrees(ctx); err != nil {
		t.Fatalf("PruneWorktrees failed: %v", err)
	}
	if _, err := os.Stat(wsFile); !os.IsNotExist(err) {
		t.Errorf("expected no .code-workspace file after PruneWorktrees, got err=%v", err)
	}
}

func TestVSCodeWorkspace_InitAndLifecycleSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	emptyDir := testutil.SetupGreenfieldDirectory(t)
	mgr := NewManager("")

	// 1. InitWithOptions(VSCode: true) creates <root>/<repo>.code-workspace with "main"
	res, err := mgr.InitWithOptions(ctx, emptyDir, InitOptions{VSCode: true})
	if err != nil {
		t.Fatalf("InitWithOptions(VSCode: true) failed: %v", err)
	}

	expectedWSPath := filepath.Join(emptyDir, filepath.Base(emptyDir)+".code-workspace")
	if res.VSCodeWorkspace != expectedWSPath {
		t.Errorf("res.VSCodeWorkspace = %q, want %q", res.VSCodeWorkspace, expectedWSPath)
	}

	ws := readVSCodeWorkspaceFile(t, expectedWSPath)
	wantFolders := []VSCodeFolder{{Name: "main", Path: "main"}}
	if !reflect.DeepEqual(ws.Folders, wantFolders) {
		t.Errorf("folders after init = %+v, want %+v", ws.Folders, wantFolders)
	}
	if ws.Settings == nil {
		t.Errorf("expected non-nil empty settings map in new .code-workspace file")
	}

	// 2. Customize top-level settings and extensions in the .code-workspace file
	customContent := `{
  "folders": [
    {
      "name": "main",
      "path": "main"
    }
  ],
  "settings": {
    "go.useLanguageServer": true
  },
  "extensions": {
    "recommendations": [
      "golang.go"
    ]
  }
}
`
	if err := os.WriteFile(expectedWSPath, []byte(customContent), 0644); err != nil {
		t.Fatalf("failed to write custom .code-workspace: %v", err)
	}

	bareMgr := NewManager(res.BareDir)

	// 3. Create worktrees: feat/102-metrics and feat/101-auth (verify deterministic sorting)
	wtMetrics, branchMetrics, err := bareMgr.CreateWorktree(ctx, "feat/102-metrics", "main", "", "")
	if err != nil {
		t.Fatalf("CreateWorktree feat/102-metrics failed: %v", err)
	}
	wtAuth, branchAuth, err := bareMgr.CreateWorktree(ctx, "feat/101-auth", "main", "", "")
	if err != nil {
		t.Fatalf("CreateWorktree feat/101-auth failed: %v", err)
	}

	ws = readVSCodeWorkspaceFile(t, expectedWSPath)
	wantFolders = []VSCodeFolder{
		{Name: "main", Path: "main"},
		{Name: "feat/101-auth", Path: "feat/101-auth"},
		{Name: "feat/102-metrics", Path: "feat/102-metrics"},
	}
	if !reflect.DeepEqual(ws.Folders, wantFolders) {
		t.Errorf("folders after create = %+v, want %+v", ws.Folders, wantFolders)
	}
	if got, ok := ws.Settings["go.useLanguageServer"].(bool); !ok || !got {
		t.Errorf("expected custom setting go.useLanguageServer=true to be preserved, got: %+v", ws.Settings)
	}
	if ws.Extensions == nil {
		t.Errorf("expected custom extensions block to be preserved, got nil")
	}

	// 4. Remove feat/101-auth and verify it is removed from folders
	if err := bareMgr.RemoveFeatureWorktree(ctx, wtAuth, branchAuth); err != nil {
		t.Fatalf("RemoveFeatureWorktree feat/101-auth failed: %v", err)
	}
	ws = readVSCodeWorkspaceFile(t, expectedWSPath)
	wantFolders = []VSCodeFolder{
		{Name: "main", Path: "main"},
		{Name: "feat/102-metrics", Path: "feat/102-metrics"},
	}
	if !reflect.DeepEqual(ws.Folders, wantFolders) {
		t.Errorf("folders after remove = %+v, want %+v", ws.Folders, wantFolders)
	}

	// 5. Delete feat/102-metrics directory via os.RemoveAll and run PruneWorktrees
	_ = branchMetrics
	if err := os.RemoveAll(wtMetrics); err != nil {
		t.Fatalf("os.RemoveAll(%s) failed: %v", wtMetrics, err)
	}
	if err := bareMgr.PruneWorktrees(ctx); err != nil {
		t.Fatalf("PruneWorktrees failed: %v", err)
	}

	ws = readVSCodeWorkspaceFile(t, expectedWSPath)
	wantFolders = []VSCodeFolder{
		{Name: "main", Path: "main"},
	}
	if !reflect.DeepEqual(ws.Folders, wantFolders) {
		t.Errorf("folders after prune = %+v, want %+v", ws.Folders, wantFolders)
	}
	if got, ok := ws.Settings["go.useLanguageServer"].(bool); !ok || !got {
		t.Errorf("expected custom setting go.useLanguageServer=true to remain preserved after prune, got: %+v", ws.Settings)
	}
}

func TestVSCodeWorkspace_BareRepairPopulatesExistingWorktrees(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rootDir, bareDir, _ := testutil.SetupPureBareWorkspace(t)
	_ = testutil.CreateFeatureWorktreeInWorkspace(t, bareDir, "feat-202-ui", "feat/202-ui")
	_ = testutil.CreateFeatureWorktreeInWorkspace(t, bareDir, "feat-101-api", "feat/101-api")

	mgr := NewManager("")
	res, err := mgr.InitWithOptions(ctx, rootDir, InitOptions{VSCode: true})
	if err != nil {
		t.Fatalf("InitWithOptions on existing Pure Bare repo failed: %v", err)
	}
	if res.Scenario != ScenarioBareRepair {
		t.Errorf("expected ScenarioBareRepair, got %s", res.Scenario)
	}

	expectedWSPath := filepath.Join(rootDir, filepath.Base(rootDir)+".code-workspace")
	ws := readVSCodeWorkspaceFile(t, expectedWSPath)
	wantFolders := []VSCodeFolder{
		{Name: "main", Path: "main"},
		{Name: "feat/101-api", Path: "feat/101-api"},
		{Name: "feat/202-ui", Path: "feat/202-ui"},
	}
	if !reflect.DeepEqual(ws.Folders, wantFolders) {
		t.Errorf("folders after bare-repair --vscode = %+v, want %+v", ws.Folders, wantFolders)
	}
}
