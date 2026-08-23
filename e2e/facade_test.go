//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/O-Marsters-1997/treepad"
	"github.com/O-Marsters-1997/treepad/internal/passthrough"
)

// The facade's whole point is naming a repo the caller is not standing in, so
// every test here runs from the e2e package directory — neither the fixture
// repo nor the worktree it cuts.

const fixtureConfig = `[sync]
include = [".env.local"]

[[hooks.post_new]]
command = "echo {{.Branch}} > {{.WorktreePath}}/post_new.marker"
`

// fixtureRepo builds a repo for a headless caller: git, a .treepad.toml, and a
// terminal that cannot be opened without failing the test.
func fixtureRepo(t *testing.T, config string) string {
	t.Helper()
	failOnTTY(t)

	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "fixture")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "facade@example.com")
	git(t, dir, "config", "user.name", "facade")
	writeFile(t, filepath.Join(dir, ".treepad.toml"), config)
	writeFile(t, filepath.Join(dir, "README.md"), "fixture\n")
	// Synced and hook-written files are gitignored in a real repo; untracked they
	// would read as uncommitted work and make every fresh worktree dirty.
	writeFile(t, filepath.Join(dir, ".gitignore"), ".env.local\npost_new.marker\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "initial")
	// Untracked, so its presence in a worktree can only come from [sync] include.
	writeFile(t, filepath.Join(dir, ".env.local"), "SECRET=1\n")

	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertNoSiblingDirs(t *testing.T, repo string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(repo))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(repo) {
			t.Errorf("unexpected directory left behind: %s", e.Name())
		}
	}
}

// failOnTTY turns any attempt to acquire a controlling terminal into a test
// failure. A library caller runs headless, so a hook that reached /dev/tty
// would block on input nobody is there to give.
func failOnTTY(t *testing.T) {
	t.Helper()
	prev := passthrough.OpenTTY
	passthrough.OpenTTY = func() *os.File {
		t.Error("a library path opened a TTY")
		return nil
	}
	t.Cleanup(func() { passthrough.OpenTTY = prev })
}

const interactiveHook = `command = "read answer"
interactive = true
`

func interactiveHookConfig(event string) string {
	return "[[hooks." + event + "]]\n" + interactiveHook
}

func TestNewCutsWorktreeInNamedRepo(t *testing.T) {
	repo := fixtureRepo(t, fixtureConfig)

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/auth",
		Base:      "main",
		RepoDir:   repo,
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if wt.Path == repo {
		t.Fatalf("Path = %q, want the created worktree, not the main one", wt.Path)
	}
	if info, err := os.Stat(wt.Path); err != nil || !info.IsDir() {
		t.Fatalf("worktree path %q is not a directory on disk: %v", wt.Path, err)
	}
	if wt.Branch != "feature/auth" {
		t.Errorf("Branch = %q, want feature/auth", wt.Branch)
	}
	if want := git(t, repo, "rev-parse", "main^{commit}"); wt.BaseSHA != want {
		t.Errorf("BaseSHA = %q, want %q", wt.BaseSHA, want)
	}
	if got := git(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature/auth" {
		t.Errorf("worktree HEAD is on %q, want feature/auth", got)
	}
}

func TestNewSyncsConfigsAndFiresHooks(t *testing.T) {
	repo := fixtureRepo(t, fixtureConfig)

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/sync",
		Base:      "main",
		RepoDir:   repo,
		OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	synced, err := os.ReadFile(filepath.Join(wt.Path, ".env.local"))
	if err != nil {
		t.Fatalf("[sync] include file missing from worktree: %v", err)
	}
	if string(synced) != "SECRET=1\n" {
		t.Errorf("synced .env.local = %q, want %q", synced, "SECRET=1\n")
	}

	marker, err := os.ReadFile(filepath.Join(wt.Path, "post_new.marker"))
	if err != nil {
		t.Fatalf("post_new hook left no marker: %v", err)
	}
	if strings.TrimSpace(string(marker)) != "feature/sync" {
		t.Errorf("post_new marker = %q, want feature/sync", marker)
	}
}

func TestNewConcurrentlyInDistinctRepos(t *testing.T) {
	repos := []string{fixtureRepo(t, fixtureConfig), fixtureRepo(t, fixtureConfig)}
	worktrees := make([]treepad.Worktree, len(repos))
	errs := make([]error, len(repos))

	var wg sync.WaitGroup
	for i, repo := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worktrees[i], errs[i] = treepad.New(context.Background(), treepad.NewOptions{
				Branch:    "feature/concurrent",
				Base:      "main",
				RepoDir:   repo,
				OutputDir: t.TempDir(),
			})
		}()
	}
	wg.Wait()

	for i, repo := range repos {
		if errs[i] != nil {
			t.Fatalf("New in repo %d: %v", i, errs[i])
		}
		want := filepath.Join(repo, ".git")
		got := git(t, worktrees[i].Path, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if got != want {
			t.Errorf("worktree %d belongs to %q, want %q", i, got, want)
		}
	}
}

func TestNewConcurrentlyInSameRepo(t *testing.T) {
	repo := fixtureRepo(t, fixtureConfig)
	branches := []string{"feature/one", "feature/two", "feature/three", "feature/four"}
	worktrees := make([]treepad.Worktree, len(branches))
	errs := make([]error, len(branches))

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, branch := range branches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			worktrees[i], errs[i] = treepad.New(context.Background(), treepad.NewOptions{
				Branch:    branch,
				Base:      "main",
				RepoDir:   repo,
				OutputDir: t.TempDir(),
			})
		}()
	}
	close(start)
	wg.Wait()

	seen := make(map[string]bool, len(branches))
	for i, branch := range branches {
		if errs[i] != nil {
			t.Fatalf("New %s: %v", branch, errs[i])
		}
		if info, err := os.Stat(worktrees[i].Path); err != nil || !info.IsDir() {
			t.Errorf("worktree for %s missing on disk: %v", branch, err)
		}
		if seen[worktrees[i].Path] {
			t.Errorf("two cuts landed on %q", worktrees[i].Path)
		}
		seen[worktrees[i].Path] = true
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	repo := fixtureRepo(t, fixtureConfig)

	tests := []struct {
		name string
		opts treepad.NewOptions
		want string
	}{
		{
			name: "empty Branch",
			opts: treepad.NewOptions{RepoDir: repo},
			want: "Branch",
		},
		{
			name: "empty RepoDir",
			opts: treepad.NewOptions{Branch: "feature/x"},
			want: "RepoDir",
		},
		{
			name: "relative RepoDir",
			opts: treepad.NewOptions{Branch: "feature/x", RepoDir: "./fixture"},
			want: "RepoDir",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := treepad.New(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not name %s", err, tt.want)
			}
		})
	}
}

func TestNewUnresolvableBase(t *testing.T) {
	repo := fixtureRepo(t, fixtureConfig)

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/nope",
		Base:      "no-such-ref",
		RepoDir:   repo,
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("want an error, got worktree %+v", wt)
	}
	if branches := git(t, repo, "branch", "--list", "feature/nope"); branches != "" {
		t.Errorf("branch was created: %q", branches)
	}
	assertNoSiblingDirs(t, repo)
}

func TestNewOutputDir(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		repo := fixtureRepo(t, fixtureConfig)
		outputDir := t.TempDir()

		if _, err := treepad.New(context.Background(), treepad.NewOptions{
			Branch:    "feature/artifact",
			Base:      "main",
			RepoDir:   repo,
			OutputDir: outputDir,
		}); err != nil {
			t.Fatalf("New: %v", err)
		}

		if entries, err := os.ReadDir(outputDir); err != nil || len(entries) != 1 {
			t.Fatalf("OutputDir holds %v (err %v), want exactly one artifact", entries, err)
		}
	})

	t.Run("unset defaults under HOME", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		repo := fixtureRepo(t, fixtureConfig)

		if _, err := treepad.New(context.Background(), treepad.NewOptions{
			Branch:  "feature/artifact",
			Base:    "main",
			RepoDir: repo,
		}); err != nil {
			t.Fatalf("New: %v", err)
		}

		defaultDir := filepath.Join(home, filepath.Base(repo)+"-workspaces")
		if entries, err := os.ReadDir(defaultDir); err != nil || len(entries) != 1 {
			t.Fatalf("%s holds %v (err %v), want exactly one artifact", defaultDir, entries, err)
		}
	})
}

func TestNewFailingPostHook(t *testing.T) {
	repo := fixtureRepo(t, "[[hooks.post_new]]\ncommand = \"exit 3\"\n")

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/bad-hook",
		Base:      "main",
		RepoDir:   repo,
		OutputDir: t.TempDir(),
	})
	if !errors.Is(err, treepad.ErrPostHook) {
		t.Fatalf("error = %v, want it to wrap ErrPostHook", err)
	}
	if wt.Branch != "feature/bad-hook" {
		t.Errorf("Branch = %q, want feature/bad-hook", wt.Branch)
	}
	if info, statErr := os.Stat(wt.Path); statErr != nil || !info.IsDir() {
		t.Errorf("worktree %q not on disk: %v", wt.Path, statErr)
	}
}

func TestRemoveDeletesWorktreeBranchAndArtifact(t *testing.T) {
	repo := fixtureRepo(t, fixtureConfig)
	outputDir := t.TempDir()

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/done",
		Base:      "main",
		RepoDir:   repo,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := treepad.Remove(context.Background(), treepad.RemoveOptions{
		Branch:    "feature/done",
		RepoDir:   repo,
		OutputDir: outputDir,
	}); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Errorf("worktree %q still on disk: %v", wt.Path, err)
	}
	if branches := git(t, repo, "branch", "--list", "feature/done"); branches != "" {
		t.Errorf("branch survived: %q", branches)
	}
	if entries, err := os.ReadDir(outputDir); err != nil || len(entries) != 0 {
		t.Errorf("OutputDir holds %v (err %v), want the artifact gone", entries, err)
	}
}

func TestRemoveUnmergedBranch(t *testing.T) {
	// What a squash merge leaves behind: the work is in main, but the branch's
	// own commit is not an ancestor of it, so git refuses a plain -d.
	setup := func(t *testing.T) (repoDir, outputDir string, wt treepad.Worktree) {
		t.Helper()
		repoDir = fixtureRepo(t, fixtureConfig)
		outputDir = t.TempDir()

		wt, err := treepad.New(context.Background(), treepad.NewOptions{
			Branch:    "feature/squashed",
			Base:      "main",
			RepoDir:   repoDir,
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		writeFile(t, filepath.Join(wt.Path, "work.txt"), "done\n")
		git(t, wt.Path, "add", "work.txt")
		git(t, wt.Path, "commit", "-m", "work")
		return repoDir, outputDir, wt
	}

	t.Run("Force false deletes nothing", func(t *testing.T) {
		repoDir, outputDir, wt := setup(t)

		err := treepad.Remove(context.Background(), treepad.RemoveOptions{
			Branch:    "feature/squashed",
			RepoDir:   repoDir,
			OutputDir: outputDir,
		})
		if err == nil {
			t.Fatal("want an error, got nil")
		}
		if branches := git(t, repoDir, "branch", "--list", "feature/squashed"); branches == "" {
			t.Error("branch was deleted")
		}
		if _, statErr := os.Stat(wt.Path); statErr != nil {
			t.Errorf("worktree %q gone: %v", wt.Path, statErr)
		}
		if entries, readErr := os.ReadDir(outputDir); readErr != nil || len(entries) != 1 {
			t.Errorf("OutputDir holds %v (err %v), want the artifact intact", entries, readErr)
		}
	})

	t.Run("Force true deletes it", func(t *testing.T) {
		repoDir, outputDir, wt := setup(t)

		if err := treepad.Remove(context.Background(), treepad.RemoveOptions{
			Branch:    "feature/squashed",
			RepoDir:   repoDir,
			OutputDir: outputDir,
			Force:     true,
		}); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if branches := git(t, repoDir, "branch", "--list", "feature/squashed"); branches != "" {
			t.Errorf("branch survived: %q", branches)
		}
		if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
			t.Errorf("worktree %q still on disk: %v", wt.Path, statErr)
		}
	})
}

func TestRemoveDirtyWorktreeIsRefusedEvenWithForce(t *testing.T) {
	repoDir := fixtureRepo(t, fixtureConfig)
	outputDir := t.TempDir()

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/wip",
		Base:      "main",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	writeFile(t, filepath.Join(wt.Path, "README.md"), "uncommitted work\n")

	err = treepad.Remove(context.Background(), treepad.RemoveOptions{
		Branch:    "feature/wip",
		RepoDir:   repoDir,
		OutputDir: outputDir,
		Force:     true,
	})
	if !errors.Is(err, treepad.ErrDirty) {
		t.Fatalf("error = %v, want it to wrap ErrDirty", err)
	}
	if _, statErr := os.Stat(wt.Path); statErr != nil {
		t.Errorf("worktree %q gone: %v", wt.Path, statErr)
	}
	if branches := git(t, repoDir, "branch", "--list", "feature/wip"); branches == "" {
		t.Error("branch was deleted")
	}
	content, readErr := os.ReadFile(filepath.Join(wt.Path, "README.md"))
	if readErr != nil || string(content) != "uncommitted work\n" {
		t.Errorf("working-tree change = %q (err %v), want it untouched", content, readErr)
	}
}

func TestRemoveAbsentBranch(t *testing.T) {
	repoDir := fixtureRepo(t, fixtureConfig)

	err := treepad.Remove(context.Background(), treepad.RemoveOptions{
		Branch:    "feature/never-existed",
		RepoDir:   repoDir,
		OutputDir: t.TempDir(),
	})
	if !errors.Is(err, treepad.ErrNotFound) {
		t.Fatalf("error = %v, want it to wrap ErrNotFound", err)
	}
	// A library caller has no tp binary to run; advice aimed at the CLI's user
	// would be noise at best.
	if strings.Contains(err.Error(), "tp sync") {
		t.Errorf("error %q tells a library caller to run tp sync", err)
	}
}

func TestRemoveRefusesMainWorktree(t *testing.T) {
	repoDir := fixtureRepo(t, fixtureConfig)

	err := treepad.Remove(context.Background(), treepad.RemoveOptions{
		Branch:    "main",
		RepoDir:   repoDir,
		OutputDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "main worktree") {
		t.Errorf("error %q does not say the main worktree is off limits", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoDir, ".git")); statErr != nil {
		t.Errorf("main worktree damaged: %v", statErr)
	}
}

func TestRemoveTwiceIsIdempotent(t *testing.T) {
	repoDir := fixtureRepo(t, fixtureConfig)
	outputDir := t.TempDir()

	if _, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/reconciled",
		Base:      "main",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	}); err != nil {
		t.Fatalf("New: %v", err)
	}

	opts := treepad.RemoveOptions{
		Branch:    "feature/reconciled",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	}
	if err := treepad.Remove(context.Background(), opts); err != nil {
		t.Fatalf("first Remove: %v", err)
	}
	if err := treepad.Remove(context.Background(), opts); !errors.Is(err, treepad.ErrNotFound) {
		t.Fatalf("second Remove = %v, want it to wrap ErrNotFound", err)
	}
}

func TestRemoveFromInsideTargetWorktree(t *testing.T) {
	repoDir := fixtureRepo(t, fixtureConfig)
	outputDir := t.TempDir()

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/inside",
		Base:      "main",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// tp remove refuses this; a library caller has no cwd to move out of.
	t.Chdir(wt.Path)

	if err := treepad.Remove(context.Background(), treepad.RemoveOptions{
		Branch:    "feature/inside",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Errorf("worktree %q still on disk: %v", wt.Path, statErr)
	}
}

func TestRemoveFailingPostHook(t *testing.T) {
	repoDir := fixtureRepo(t, "[[hooks.post_remove]]\ncommand = \"exit 3\"\n")
	outputDir := t.TempDir()

	wt, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/bad-post-remove",
		Base:      "main",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = treepad.Remove(context.Background(), treepad.RemoveOptions{
		Branch:    "feature/bad-post-remove",
		RepoDir:   repoDir,
		OutputDir: outputDir,
	})
	if !errors.Is(err, treepad.ErrPostHook) {
		t.Fatalf("error = %v, want it to wrap ErrPostHook", err)
	}
	if _, statErr := os.Stat(wt.Path); !os.IsNotExist(statErr) {
		t.Errorf("worktree %q still on disk: %v", wt.Path, statErr)
	}
	if branches := git(t, repoDir, "branch", "--list", "feature/bad-post-remove"); branches != "" {
		t.Errorf("branch survived: %q", branches)
	}
	if entries, readErr := os.ReadDir(outputDir); readErr != nil || len(entries) != 0 {
		t.Errorf("OutputDir holds %v (err %v), want the artifact gone", entries, readErr)
	}
}

func TestRemoveRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opts treepad.RemoveOptions
		want string
	}{
		{
			name: "empty Branch",
			opts: treepad.RemoveOptions{RepoDir: "/tmp"},
			want: "Branch",
		},
		{
			name: "empty RepoDir",
			opts: treepad.RemoveOptions{Branch: "feature/x"},
			want: "RepoDir",
		},
		{
			name: "relative RepoDir",
			opts: treepad.RemoveOptions{Branch: "feature/x", RepoDir: "./fixture"},
			want: "RepoDir",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := treepad.Remove(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not name %s", err, tt.want)
			}
		})
	}
}

func TestNewRefusesInteractiveCutHooks(t *testing.T) {
	// Every event a cut fires, the sync sandwich inside it included: a refusal
	// that only arrived after git worktree add would leave debris behind.
	for _, event := range []string{"pre_new", "post_new", "pre_sync", "post_sync"} {
		t.Run(event, func(t *testing.T) {
			repo := fixtureRepo(t, interactiveHookConfig(event))

			wt, err := treepad.New(context.Background(), treepad.NewOptions{
				Branch:    "feature/prompt",
				Base:      "main",
				RepoDir:   repo,
				OutputDir: t.TempDir(),
			})
			if !errors.Is(err, treepad.ErrInteractiveHook) {
				t.Fatalf("error = %v, want it to wrap ErrInteractiveHook", err)
			}
			if wt != (treepad.Worktree{}) {
				t.Errorf("Worktree = %+v, want the zero value", wt)
			}
			if branches := git(t, repo, "branch", "--list", "feature/prompt"); branches != "" {
				t.Errorf("branch was created: %q", branches)
			}
			assertNoSiblingDirs(t, repo)
		})
	}
}

func TestRemoveRefusesInteractiveHooks(t *testing.T) {
	for _, event := range []string{"pre_remove", "post_remove"} {
		t.Run(event, func(t *testing.T) {
			repo := fixtureRepo(t, interactiveHookConfig(event))
			outputDir := t.TempDir()

			// The cut goes through: a remove hook is not a cut hook, so each
			// operation refuses only over the events it actually fires.
			wt, err := treepad.New(context.Background(), treepad.NewOptions{
				Branch:    "feature/prompt",
				Base:      "main",
				RepoDir:   repo,
				OutputDir: outputDir,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			err = treepad.Remove(context.Background(), treepad.RemoveOptions{
				Branch:    "feature/prompt",
				RepoDir:   repo,
				OutputDir: outputDir,
			})
			if !errors.Is(err, treepad.ErrInteractiveHook) {
				t.Fatalf("error = %v, want it to wrap ErrInteractiveHook", err)
			}
			if _, statErr := os.Stat(wt.Path); statErr != nil {
				t.Errorf("worktree %q gone: %v", wt.Path, statErr)
			}
			if branches := git(t, repo, "branch", "--list", "feature/prompt"); branches == "" {
				t.Error("branch was deleted")
			}
			if entries, readErr := os.ReadDir(outputDir); readErr != nil || len(entries) != 1 {
				t.Errorf("OutputDir holds %v (err %v), want the artifact intact", entries, readErr)
			}
		})
	}
}

func TestNewInteractiveHookExcludedByFilter(t *testing.T) {
	for _, filter := range []string{`only = ["release/*"]`, `except = ["feature/**"]`} {
		t.Run(filter, func(t *testing.T) {
			repo := fixtureRepo(t, "[[hooks.pre_new]]\n"+interactiveHook+filter+"\n")

			wt, err := treepad.New(context.Background(), treepad.NewOptions{
				Branch:    "feature/unaffected",
				Base:      "main",
				RepoDir:   repo,
				OutputDir: t.TempDir(),
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if info, statErr := os.Stat(wt.Path); statErr != nil || !info.IsDir() {
				t.Errorf("worktree %q not on disk: %v", wt.Path, statErr)
			}
		})
	}
}

// A reconcile loop reads ErrNotFound and ErrDirty as state it already knows
// about, so a repo whose remove hooks want a human must not turn either into a
// refusal the caller has no rule for.
func TestRemoveInteractiveHookDoesNotShadowTheSentinels(t *testing.T) {
	t.Run("ErrNotFound", func(t *testing.T) {
		repo := fixtureRepo(t, interactiveHookConfig("pre_remove"))

		err := treepad.Remove(context.Background(), treepad.RemoveOptions{
			Branch:    "feature/never-existed",
			RepoDir:   repo,
			OutputDir: t.TempDir(),
		})
		if !errors.Is(err, treepad.ErrNotFound) {
			t.Fatalf("error = %v, want it to wrap ErrNotFound", err)
		}
	})

	t.Run("ErrDirty", func(t *testing.T) {
		repo := fixtureRepo(t, interactiveHookConfig("pre_remove"))
		outputDir := t.TempDir()

		wt, err := treepad.New(context.Background(), treepad.NewOptions{
			Branch:    "feature/wip",
			Base:      "main",
			RepoDir:   repo,
			OutputDir: outputDir,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		writeFile(t, filepath.Join(wt.Path, "README.md"), "uncommitted work\n")

		err = treepad.Remove(context.Background(), treepad.RemoveOptions{
			Branch:    "feature/wip",
			RepoDir:   repo,
			OutputDir: outputDir,
		})
		if !errors.Is(err, treepad.ErrDirty) {
			t.Fatalf("error = %v, want it to wrap ErrDirty", err)
		}
	})
}

func TestNewRefusalPrecedesTheWholeEvent(t *testing.T) {
	// The quiet hook is declared first, so a refusal made per entry rather than
	// per event would already have run it.
	repo := fixtureRepo(t, `[[hooks.pre_new]]
command = "touch quiet.marker"

[[hooks.pre_new]]
command = "read answer"
interactive = true
`)

	if _, err := treepad.New(context.Background(), treepad.NewOptions{
		Branch:    "feature/prompt",
		Base:      "main",
		RepoDir:   repo,
		OutputDir: t.TempDir(),
	}); !errors.Is(err, treepad.ErrInteractiveHook) {
		t.Fatalf("error = %v, want it to wrap ErrInteractiveHook", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "quiet.marker")); !os.IsNotExist(err) {
		t.Errorf("the non-interactive hook in the same event ran: %v", err)
	}
}
