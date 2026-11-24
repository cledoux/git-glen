package worktree

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"git-glen/internal/testutil"
)

func TestSyncAgentLinks_CreatesAndRepairsRelativeSymlinks(t *testing.T) {
	baseDir := t.TempDir()
	mainDir := filepath.Join(baseDir, "main")
	if err := os.MkdirAll(filepath.Join(mainDir, ".agents", "rules"), 0o755); err != nil {
		t.Fatalf("mkdir main/.agents/rules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, ".agents", "rules", "sdd.md"), []byte("# rules\n"), 0o644); err != nil {
		t.Fatalf("write sdd.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "CLAUDE.md"), []byte("# Claude\n"), 0o644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	linked, err := SyncAgentLinks(baseDir, mainDir)
	if err != nil {
		t.Fatalf("SyncAgentLinks failed: %v", err)
	}
	want := []string{".agents", "AGENTS.md", "CLAUDE.md"}
	if !reflect.DeepEqual(linked, want) {
		t.Fatalf("linked = %v, want %v", linked, want)
	}

	for _, name := range want {
		dst := filepath.Join(baseDir, name)
		target, err := os.Readlink(dst)
		if err != nil {
			t.Fatalf("Readlink(%s) failed: %v", dst, err)
		}
		wantTarget := filepath.Join("main", name)
		if target != wantTarget {
			t.Errorf("symlink %s -> %q, want %q", name, target, wantTarget)
		}
	}

	// Verify content resolves through the symlinked directory.
	if _, err := os.Stat(filepath.Join(baseDir, ".agents", "rules", "sdd.md")); err != nil {
		t.Errorf("expected rule file to resolve through .agents symlink: %v", err)
	}

	// Entries absent from main/ must not be created.
	for _, absent := range []string{".claude", "GEMINI.md"} {
		if _, err := os.Lstat(filepath.Join(baseDir, absent)); !os.IsNotExist(err) {
			t.Errorf("expected no symlink for absent %s, got err=%v", absent, err)
		}
	}

	// Second call is idempotent and returns the same linked entries.
	linked2, err := SyncAgentLinks(baseDir, mainDir)
	if err != nil {
		t.Fatalf("second SyncAgentLinks failed: %v", err)
	}
	if !reflect.DeepEqual(linked2, want) {
		t.Errorf("second linked = %v, want %v", linked2, want)
	}
}

func TestSyncAgentLinks_PreservesRealFilesAndCustomSymlinks(t *testing.T) {
	baseDir := t.TempDir()
	mainDir := filepath.Join(baseDir, "main")
	customDir := filepath.Join(baseDir, "custom-agents")
	if err := os.MkdirAll(filepath.Join(mainDir, ".agents"), 0o755); err != nil {
		t.Fatalf("mkdir main/.agents: %v", err)
	}
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("mkdir custom-agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "AGENTS.md"), []byte("# Main Agents\n"), 0o644); err != nil {
		t.Fatalf("write main/AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "GEMINI.md"), []byte("# Main Gemini\n"), 0o644); err != nil {
		t.Fatalf("write main/GEMINI.md: %v", err)
	}

	// Real file at root must NOT be clobbered.
	if err := os.WriteFile(filepath.Join(baseDir, "AGENTS.md"), []byte("# Custom Root\n"), 0o644); err != nil {
		t.Fatalf("write root AGENTS.md: %v", err)
	}
	// Valid custom symlink pointing elsewhere must NOT be clobbered.
	if err := os.Symlink(customDir, filepath.Join(baseDir, ".agents")); err != nil {
		t.Fatalf("symlink .agents: %v", err)
	}
	// Broken symlink at root SHOULD be repaired to point into main/.
	if err := os.Symlink("nonexistent/GEMINI.md", filepath.Join(baseDir, "GEMINI.md")); err != nil {
		t.Fatalf("symlink broken GEMINI.md: %v", err)
	}

	linked, err := SyncAgentLinks(baseDir, mainDir)
	if err != nil {
		t.Fatalf("SyncAgentLinks failed: %v", err)
	}
	if !reflect.DeepEqual(linked, []string{"GEMINI.md"}) {
		t.Errorf("linked = %v, want [GEMINI.md]", linked)
	}

	// Verify real root AGENTS.md was untouched.
	data, err := os.ReadFile(filepath.Join(baseDir, "AGENTS.md"))
	if err != nil || string(data) != "# Custom Root\n" {
		t.Errorf("root AGENTS.md was modified: data=%q err=%v", string(data), err)
	}
	// Verify custom .agents symlink was untouched.
	gotCustom, err := os.Readlink(filepath.Join(baseDir, ".agents"))
	if err != nil || gotCustom != customDir {
		t.Errorf("custom .agents symlink was modified: target=%q err=%v", gotCustom, err)
	}
	// Verify broken GEMINI.md symlink was repaired.
	gotGemini, err := os.Readlink(filepath.Join(baseDir, "GEMINI.md"))
	if err != nil || gotGemini != filepath.Join("main", "GEMINI.md") {
		t.Errorf("GEMINI.md symlink = %q, want %q (err=%v)", gotGemini, filepath.Join("main", "GEMINI.md"), err)
	}
}

func TestSyncAgentLinks_PrunesStaleManagedSymlink(t *testing.T) {
	baseDir := t.TempDir()
	mainDir := filepath.Join(baseDir, "main")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatalf("mkdir main: %v", err)
	}

	// Simulate .agents having been removed from main/ after glen previously linked it.
	if err := os.Symlink(filepath.Join("main", ".agents"), filepath.Join(baseDir, ".agents")); err != nil {
		t.Fatalf("create stale symlink: %v", err)
	}
	// An unrelated broken symlink pointing elsewhere must NOT be deleted.
	if err := os.Symlink("elsewhere/CLAUDE.md", filepath.Join(baseDir, "CLAUDE.md")); err != nil {
		t.Fatalf("create unrelated symlink: %v", err)
	}

	linked, err := SyncAgentLinks(baseDir, mainDir)
	if err != nil {
		t.Fatalf("SyncAgentLinks failed: %v", err)
	}
	if len(linked) != 0 {
		t.Errorf("expected no linked entries, got %v", linked)
	}
	if _, err := os.Lstat(filepath.Join(baseDir, ".agents")); !os.IsNotExist(err) {
		t.Errorf("expected stale glen-managed .agents symlink to be removed, got err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(baseDir, "CLAUDE.md")); err != nil {
		t.Errorf("expected unrelated CLAUDE.md symlink to remain untouched, got err=%v", err)
	}
}

func TestInitAndRepair_SyncsAgentLinksEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("ConvertStandardClone", func(t *testing.T) {
		cloneDir := testutil.SetupStandardClone(t, false)
		if err := os.MkdirAll(filepath.Join(cloneDir, ".agents", "rules"), 0o755); err != nil {
			t.Fatalf("mkdir .agents/rules: %v", err)
		}
		if err := os.WriteFile(filepath.Join(cloneDir, ".agents", "rules", "sdd.md"), []byte("# SDD\n"), 0o644); err != nil {
			t.Fatalf("write sdd.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(cloneDir, "AGENTS.md"), []byte("# Agents\n"), 0o644); err != nil {
			t.Fatalf("write AGENTS.md: %v", err)
		}
		testutil.RunGit(t, cloneDir, "add", ".agents", "AGENTS.md")
		testutil.RunGit(t, cloneDir, "commit", "-m", "Add agent configs")

		mgr := NewManager("")
		res, err := mgr.Init(ctx, cloneDir)
		if err != nil {
			t.Fatalf("Init (clone-convert) failed: %v", err)
		}
		want := []string{".agents", "AGENTS.md"}
		if !reflect.DeepEqual(res.AgentLinks, want) {
			t.Errorf("res.AgentLinks = %v, want %v", res.AgentLinks, want)
		}
		for _, name := range want {
			got, err := os.Readlink(filepath.Join(cloneDir, name))
			if err != nil {
				t.Fatalf("Readlink(%s) failed: %v", name, err)
			}
			if got != filepath.Join("main", name) {
				t.Errorf("%s -> %q, want %q", name, got, filepath.Join("main", name))
			}
		}
	})

	t.Run("RemoteCloneAndRepair", func(t *testing.T) {
		seedDir := t.TempDir()
		testutil.RunGit(t, seedDir, "init", "-b", "main")
		testutil.RunGit(t, seedDir, "config", "user.name", "Seed Author")
		testutil.RunGit(t, seedDir, "config", "user.email", "seed@example.com")
		if err := os.MkdirAll(filepath.Join(seedDir, ".agents", "rules"), 0o755); err != nil {
			t.Fatalf("mkdir .agents/rules: %v", err)
		}
		_ = os.WriteFile(filepath.Join(seedDir, ".agents", "rules", "rules.md"), []byte("# Rules\n"), 0o644)
		testutil.RunGit(t, seedDir, "add", ".agents")
		testutil.RunGit(t, seedDir, "commit", "-m", "Seed commit")

		targetDir := filepath.Join(t.TempDir(), "cloned")
		mgr := NewManager("")
		res, err := mgr.InitBareRepo(ctx, seedDir, targetDir)
		if err != nil {
			t.Fatalf("InitBareRepo failed: %v", err)
		}
		if !reflect.DeepEqual(res.AgentLinks, []string{".agents"}) {
			t.Errorf("InitBareRepo AgentLinks = %v, want [.agents]", res.AgentLinks)
		}

		// Delete root symlink and verify RepairWorkspace recreates it.
		if err := os.Remove(filepath.Join(targetDir, ".agents")); err != nil {
			t.Fatalf("remove .agents symlink: %v", err)
		}
		resRepair, err := mgr.RepairWorkspace(ctx, targetDir)
		if err != nil {
			t.Fatalf("RepairWorkspace failed: %v", err)
		}
		if !reflect.DeepEqual(resRepair.AgentLinks, []string{".agents"}) {
			t.Errorf("RepairWorkspace AgentLinks = %v, want [.agents]", resRepair.AgentLinks)
		}
		got, err := os.Readlink(filepath.Join(targetDir, ".agents"))
		if err != nil || got != filepath.Join("main", ".agents") {
			t.Errorf("repaired .agents -> %q, want %q (err=%v)", got, filepath.Join("main", ".agents"), err)
		}
	})
}
