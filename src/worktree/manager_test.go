package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupBareRepoTestEnv creates a pure bare repository topology (.bare + main) for testing.
func setupBareRepoTestEnv(t *testing.T) (string, string, *Manager) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	rootDir := t.TempDir()
	bareDir := filepath.Join(rootDir, ".bare")
	seedDir := filepath.Join(rootDir, "seed")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 1. Create a seed repository to generate initial commit
	if err := os.MkdirAll(seedDir, 0755); err != nil {
		t.Fatalf("failed to create seed dir: %v", err)
	}
	cmd := exec.CommandContext(ctx, "git", "init", "-b", "main", seedDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init seed failed: %v (%s)", err, out)
	}

	cmd = exec.CommandContext(ctx, "git", "config", "user.name", "Test Setup")
	cmd.Dir = seedDir
	_ = cmd.Run()
	cmd = exec.CommandContext(ctx, "git", "config", "user.email", "setup@example.com")
	cmd.Dir = seedDir
	_ = cmd.Run()

	cmd = exec.CommandContext(ctx, "git", "commit", "--allow-empty", "-m", "Initial root commit")
	cmd.Dir = seedDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed commit failed: %v (%s)", err, out)
	}

	// 2. Clone seed repository as bare into .bare
	cmd = exec.CommandContext(ctx, "git", "clone", "--bare", seedDir, bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone bare failed: %v (%s)", err, out)
	}

	// 3. Configure bare repository with standard ADR-0003 config
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure remote.origin.fetch failed: %v (%s)", err, out)
	}
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "config", "extensions.worktreeConfig", "true")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure extensions.worktreeConfig failed: %v (%s)", err, out)
	}
	// 4. Create primary 'main' linked worktree
	mainDir := filepath.Join(rootDir, "main")
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "worktree", "add", mainDir, "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add main worktree failed: %v (%s)", err, out)
	}

	cmd = exec.CommandContext(ctx, "git", "-C", mainDir, "config", "--worktree", "core.bare", "false")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure main core.bare false failed: %v (%s)", err, out)
	}

	mgr := NewManager(bareDir)
	return rootDir, bareDir, mgr
}

// TestBareWorktreeLifecycleAndIsolation verifies the complete lifecycle and git config
// isolation of ephemeral worktrees linked to a pure bare repository (.bare).
// Spec: openspec/specs/orchestration/spec.md (REQ-0003, REQ-0004)
// ADR: docs/adrs/ADR-0003-pure-bare-repository-topology.md
func TestBareWorktreeLifecycleAndIsolation(t *testing.T) {
	rootDir, bareDir, mgr := setupBareRepoTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify main worktree gitdir points into .bare
	mainGitFile := filepath.Join(rootDir, "main", ".git")
	gitPointerBytes, err := os.ReadFile(mainGitFile)
	if err != nil {
		t.Fatalf("expected main/.git pointer file to exist: %v", err)
	}
	gitPointer := string(gitPointerBytes)
	if !strings.Contains(gitPointer, ".bare") && !strings.Contains(gitPointer, "worktrees") {
		t.Errorf("main/.git should reference .bare/worktrees, got: %s", gitPointer)
	}

	// GIVEN: Ephemeral worktrees for Archer (feat/101-auth) and Beckett (feat/102-metrics)
	archerName := "Archer"
	archerEmail := "user+agent-archer@example.com"
	wtArcher, branchArcher, err := mgr.CreateFeatureWorktree(ctx, 101, "auth", "main", archerName, archerEmail)
	if err != nil {
		t.Fatalf("CreateFeatureWorktree for Archer failed: %v", err)
	}
	expectedArcherBranch := "feat/101-auth"
	expectedArcherPath := filepath.Join(rootDir, "feat", "101-auth")
	if branchArcher != expectedArcherBranch {
		t.Errorf("expected branch %s, got %s", expectedArcherBranch, branchArcher)
	}
	if wtArcher != expectedArcherPath {
		t.Errorf("expected worktree path %s, got %s", expectedArcherPath, wtArcher)
	}

	beckettName := "Beckett"
	beckettEmail := "user+agent-beckett@example.com"
	wtBeckett, branchBeckett, err := mgr.CreateFeatureWorktree(ctx, 102, "metrics", "main", beckettName, beckettEmail)
	if err != nil {
		t.Fatalf("CreateFeatureWorktree for Beckett failed: %v", err)
	}
	expectedBeckettBranch := "feat/102-metrics"
	expectedBeckettPath := filepath.Join(rootDir, "feat", "102-metrics")
	if branchBeckett != expectedBeckettBranch {
		t.Errorf("expected branch %s, got %s", expectedBeckettBranch, branchBeckett)
	}
	if wtBeckett != expectedBeckettPath {
		t.Errorf("expected worktree path %s, got %s", expectedBeckettPath, wtBeckett)
	}

	// WHEN: Inspecting --worktree scoped user.name and user.email config
	nameArcher, err := mgr.runGit(ctx, wtArcher, "config", "user.name")
	if err != nil || nameArcher != archerName {
		t.Errorf("expected Archer user.name '%s', got '%s' (err: %v)", archerName, nameArcher, err)
	}
	emailArcher, err := mgr.runGit(ctx, wtArcher, "config", "user.email")
	if err != nil || emailArcher != archerEmail {
		t.Errorf("expected Archer user.email '%s', got '%s' (err: %v)", archerEmail, emailArcher, err)
	}

	nameBeckett, err := mgr.runGit(ctx, wtBeckett, "config", "user.name")
	if err != nil || nameBeckett != beckettName {
		t.Errorf("expected Beckett user.name '%s', got '%s' (err: %v)", beckettName, nameBeckett, err)
	}
	emailBeckett, err := mgr.runGit(ctx, wtBeckett, "config", "user.email")
	if err != nil || emailBeckett != beckettEmail {
		t.Errorf("expected Beckett user.email '%s', got '%s' (err: %v)", beckettEmail, emailBeckett, err)
	}

	// Verify per-worktree config files exist in .bare/worktrees/
	archerConfigWorktree := filepath.Join(bareDir, "worktrees", "101-auth", "config.worktree")
	if _, err := os.Stat(archerConfigWorktree); err != nil {
		t.Errorf("expected config.worktree at %s: %v", archerConfigWorktree, err)
	}
	beckettConfigWorktree := filepath.Join(bareDir, "worktrees", "102-metrics", "config.worktree")
	if _, err := os.Stat(beckettConfigWorktree); err != nil {
		t.Errorf("expected config.worktree at %s: %v", beckettConfigWorktree, err)
	}

	// THEN: Commits authored in each worktree reflect respective agent identity without colliding
	if err := os.WriteFile(filepath.Join(wtArcher, "auth.go"), []byte("package auth\n"), 0644); err != nil {
		t.Fatalf("failed to create file in Archer worktree: %v", err)
	}
	if _, err := mgr.runGit(ctx, wtArcher, "add", "auth.go"); err != nil {
		t.Fatalf("git add in Archer worktree failed: %v", err)
	}
	if _, err := mgr.runGit(ctx, wtArcher, "commit", "-m", "feat: add auth module"); err != nil {
		t.Fatalf("git commit in Archer worktree failed: %v", err)
	}
	archerAuthor, err := mgr.runGit(ctx, wtArcher, "log", "-1", "--format=%an <%ae>")
	if err != nil || archerAuthor != "Archer <user+agent-archer@example.com>" {
		t.Errorf("expected Archer commit author 'Archer <user+agent-archer@example.com>', got '%s' (err: %v)", archerAuthor, err)
	}

	if err := os.WriteFile(filepath.Join(wtBeckett, "metrics.go"), []byte("package metrics\n"), 0644); err != nil {
		t.Fatalf("failed to create file in Beckett worktree: %v", err)
	}
	if _, err := mgr.runGit(ctx, wtBeckett, "add", "metrics.go"); err != nil {
		t.Fatalf("git add in Beckett worktree failed: %v", err)
	}
	if _, err := mgr.runGit(ctx, wtBeckett, "commit", "-m", "feat: add metrics module"); err != nil {
		t.Fatalf("git commit in Beckett worktree failed: %v", err)
	}
	beckettAuthor, err := mgr.runGit(ctx, wtBeckett, "log", "-1", "--format=%an <%ae>")
	if err != nil || beckettAuthor != "Beckett <user+agent-beckett@example.com>" {
		t.Errorf("expected Beckett commit author 'Beckett <user+agent-beckett@example.com>', got '%s' (err: %v)", beckettAuthor, err)
	}

	// WHEN: Listing worktrees
	worktrees, err := mgr.ListWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	// In a bare repository topology, ListWorktrees includes the bare root entry + 3 linked worktrees (main, auth, metrics)
	if len(worktrees) != 4 {
		t.Errorf("expected 4 worktree entries (1 bare root + 3 linked worktrees), got %d: %+v", len(worktrees), worktrees)
	}

	// THEN: RemoveFeatureWorktree cleans up directory and prunes .bare/worktrees/ metadata
	if err := mgr.RemoveFeatureWorktree(ctx, wtArcher, branchArcher); err != nil {
		t.Fatalf("RemoveFeatureWorktree for Archer failed: %v", err)
	}

	// Verify worktree directory is deleted
	if _, err := os.Stat(wtArcher); !os.IsNotExist(err) {
		t.Errorf("expected worktree directory %s to be deleted after remove, err: %v", wtArcher, err)
	}

	// Verify .bare/worktrees/ metadata is cleaned up
	archerMetaDir := filepath.Join(bareDir, "worktrees", "101-auth")
	if _, err := os.Stat(archerMetaDir); !os.IsNotExist(err) {
		t.Errorf("expected .bare worktree metadata dir %s to be deleted or pruned", archerMetaDir)
	}

	// Verify local feature branch is deleted
	if _, err := mgr.runGit(ctx, bareDir, "rev-parse", "--verify", branchArcher); err == nil {
		t.Errorf("expected local feature branch %s to be deleted from bare repo", branchArcher)
	}

	// Verify ListWorktrees now shows remaining worktrees (1 bare root + main + feat/102-metrics)
	worktreesAfter, err := mgr.ListWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListWorktrees after remove failed: %v", err)
	}
	if len(worktreesAfter) != 3 {
		t.Errorf("expected 3 worktrees after removal (1 bare root + 2 linked worktrees), got %d: %+v", len(worktreesAfter), worktreesAfter)
	}
}

// TestBareWorktreeDefensivePrune verifies that interrupted or dirty worktree removals
// (e.g. process killed before clean teardown) can be recovered via defensive prune.
// Spec: openspec/specs/orchestration/spec.md (REQ-0004)
// ADR: docs/adrs/ADR-0003-pure-bare-repository-topology.md Section 4 (Cleanup, Lock Recovery & Pruning)
func TestBareWorktreeDefensivePrune(t *testing.T) {
	_, bareDir, mgr := setupBareRepoTestEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Provision feature worktree for Calvin
	wtDir, branch, err := mgr.CreateFeatureWorktree(ctx, 103, "interrupted-task", "main", "Calvin", "user+agent-calvin@example.com")
	if err != nil {
		t.Fatalf("CreateFeatureWorktree failed: %v", err)
	}

	// Verify metadata exists in .bare/worktrees/
	metaDir := filepath.Join(bareDir, "worktrees", "103-interrupted-task")
	if _, err := os.Stat(metaDir); os.IsNotExist(err) {
		t.Fatalf("expected worktree metadata dir %s to exist before simulate crash", metaDir)
	}

	// Simulate abrupt workspace deletion (e.g. OS crash or manual rm -rf without git worktree remove)
	if err := os.RemoveAll(wtDir); err != nil {
		t.Fatalf("failed to remove worktree directory for dirty test: %v", err)
	}

	// Metadata directory should still exist prior to prune (dangling reference)
	if _, err := os.Stat(metaDir); os.IsNotExist(err) {
		t.Fatalf("expected dangling metadata dir %s to still exist before prune", metaDir)
	}

	// WHEN: PruneWorktrees is executed
	if err := mgr.PruneWorktrees(ctx); err != nil {
		t.Fatalf("PruneWorktrees failed: %v", err)
	}

	// THEN: Dangling metadata in .bare/worktrees/ SHALL be pruned
	if _, err := os.Stat(metaDir); !os.IsNotExist(err) {
		t.Errorf("expected dangling metadata directory %s to be pruned after PruneWorktrees", metaDir)
	}

	// And ListWorktrees SHALL no longer report the pruned worktree
	worktrees, err := mgr.ListWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	for _, wt := range worktrees {
		if wt.Path == wtDir || wt.Branch == branch {
			t.Errorf("ListWorktrees still includes pruned worktree: %+v", wt)
		}
	}
}

// TestEnsureBareRepoConfig verifies that EnsureBareRepoConfig configures both
// extensions.worktreeConfig and remote.origin.fetch on a bare repository database.
// Spec: openspec/specs/orchestration/spec.md (REQ-0003)
// ADR: docs/adrs/ADR-0003-pure-bare-repository-topology.md Section 1 & 2
func TestEnsureBareRepoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	bareDir := filepath.Join(tmpDir, ".bare")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Initialize raw unconfigured bare repository
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v (%s)", err, out)
	}

	// Add remote origin
	cmd = exec.CommandContext(ctx, "git", "--git-dir="+bareDir, "remote", "add", "origin", "https://github.com/example-org/git-glen.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add failed: %v (%s)", err, out)
	}

	mgr := NewManager(bareDir)

	// WHEN: EnsureBareRepoConfig is called
	if err := mgr.EnsureBareRepoConfig(ctx); err != nil {
		t.Fatalf("EnsureBareRepoConfig failed: %v", err)
	}

	// THEN: extensions.worktreeConfig SHALL be true
	val, err := mgr.runGit(ctx, bareDir, "config", "--get", "extensions.worktreeConfig")
	if err != nil || val != "true" {
		t.Errorf("expected extensions.worktreeConfig to be 'true', got '%s' (err: %v)", val, err)
	}

	// AND remote.origin.fetch SHALL be '+refs/heads/*:refs/remotes/origin/*'
	fetchRefspec, err := mgr.runGit(ctx, bareDir, "config", "--get", "remote.origin.fetch")
	expectedRefspec := "+refs/heads/*:refs/remotes/origin/*"
	if err != nil || fetchRefspec != expectedRefspec {
		t.Errorf("expected remote.origin.fetch to be '%s', got '%s' (err: %v)", expectedRefspec, fetchRefspec, err)
	}

	// THEN: Idempotent execution: Calling EnsureBareRepoConfig multiple times should succeed without error or duplicating configs
	if err := mgr.EnsureBareRepoConfig(ctx); err != nil {
		t.Fatalf("subsequent EnsureBareRepoConfig call failed: %v", err)
	}

	allRefspecs, err := mgr.runGit(ctx, bareDir, "config", "--get-all", "remote.origin.fetch")
	if err != nil {
		t.Fatalf("failed to query all remote.origin.fetch values: %v", err)
	}
	refspecLines := strings.Split(strings.TrimSpace(allRefspecs), "\n")
	if len(refspecLines) != 1 || refspecLines[0] != expectedRefspec {
		t.Errorf("expected exactly 1 fetch refspec entry, got %d: %v", len(refspecLines), refspecLines)
	}
}

// TestWorktreeLifecycleAndConfigIsolation preserves regression tests for standard non-bare setups.
func TestWorktreeLifecycleAndConfigIsolation(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "init", "-b", "main", repoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v (%s)", err, out)
	}

	cmd = exec.CommandContext(ctx, "git", "commit", "--allow-empty", "-m", "initial")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v (%s)", err, out)
	}

	mgr := NewManager(repoDir)

	wtAlice, branchAlice, err := mgr.CreateFeatureWorktree(ctx, 101, "scanner-feature", "main", "Alice", "user+agent-alice@example.com")
	if err != nil {
		t.Fatalf("CreateFeatureWorktree for Alice failed: %v", err)
	}
	if branchAlice != "feat/101-scanner-feature" {
		t.Errorf("expected branch feat/101-scanner-feature, got %s", branchAlice)
	}

	wtBob, branchBob, err := mgr.CreateFeatureWorktree(ctx, 102, "report-feature", "main", "Bob", "user+agent-bob@example.com")
	if err != nil {
		t.Fatalf("CreateFeatureWorktree for Bob failed: %v", err)
	}
	if branchBob != "feat/102-report-feature" {
		t.Errorf("expected branch feat/102-report-feature, got %s", branchBob)
	}

	nameAlice, err := mgr.runGit(ctx, wtAlice, "config", "user.name")
	if err != nil || nameAlice != "Alice" {
		t.Errorf("expected Alice in wtAlice, got %s (err: %v)", nameAlice, err)
	}
	emailAlice, err := mgr.runGit(ctx, wtAlice, "config", "user.email")
	if err != nil || emailAlice != "user+agent-alice@example.com" {
		t.Errorf("expected user+agent-alice@example.com in wtAlice, got %s (err: %v)", emailAlice, err)
	}

	nameBob, err := mgr.runGit(ctx, wtBob, "config", "user.name")
	if err != nil || nameBob != "Bob" {
		t.Errorf("expected Bob in wtBob, got %s (err: %v)", nameBob, err)
	}
	emailBob, err := mgr.runGit(ctx, wtBob, "config", "user.email")
	if err != nil || emailBob != "user+agent-bob@example.com" {
		t.Errorf("expected user+agent-bob@example.com in wtBob, got %s (err: %v)", emailBob, err)
	}

	worktrees, err := mgr.ListWorktrees(ctx)
	if err != nil {
		t.Fatalf("ListWorktrees failed: %v", err)
	}
	if len(worktrees) < 3 {
		t.Errorf("expected at least 3 worktrees, got %d", len(worktrees))
	}

	if err := mgr.RemoveFeatureWorktree(ctx, wtAlice, branchAlice); err != nil {
		t.Fatalf("RemoveFeatureWorktree failed: %v", err)
	}

	if _, err := os.Stat(wtAlice); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted after remove", wtAlice)
	}
}
