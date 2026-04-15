//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"tp": func() { os.Exit(Run(os.Args, os.Stdout, os.Stderr)) },
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars, "HOME="+env.WorkDir)
			return nil
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"tp-init-repo":    cmdTPInitRepo,
			"tp-add-worktree": cmdTPAddWorktree,
		},
	})
}

// cmdTPInitRepo creates a git repository in a "repo" subdirectory and changes
// the testscript working directory into it. Each txtar test calls this as its
// first command to get a clean, isolated git repo fixture.
func cmdTPInitRepo(ts *testscript.TestScript, neg bool, _ []string) {
	if neg {
		ts.Fatalf("unsupported: ! tp-init-repo")
	}
	dir := ts.MkAbs("repo")
	ts.Check(os.MkdirAll(dir, 0o755))

	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			ts.Fatalf("%s: %v\n%s", args, err, out)
		}
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test User")
	ts.Check(os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644))
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	ts.Check(ts.Chdir("repo"))
}

// cmdTPAddWorktree adds a git worktree at a sibling directory, mirroring the
// path convention tp new uses: <parent>/<repo-slug>-<name>. Useful for seeding
// multi-worktree scenarios such as prune and status.
func cmdTPAddWorktree(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("unsupported: ! tp-add-worktree")
	}
	if len(args) < 1 {
		ts.Fatalf("usage: tp-add-worktree <name>")
	}
	name := args[0]
	cwd := ts.MkAbs(".")
	parent := filepath.Dir(cwd)
	repoSlug := filepath.Base(cwd)
	wtPath := filepath.Join(parent, repoSlug+"-"+name)

	cmd := exec.Command("git", "worktree", "add", wtPath, "-b", name)
	cmd.Dir = cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		ts.Fatalf("git worktree add: %v\n%s", err, out)
	}
}
