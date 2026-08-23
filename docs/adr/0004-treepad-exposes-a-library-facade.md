# 4. Treepad exposes a library facade alongside the CLI

Date: 2026-08-23

## Status

Accepted

## Context

Treepad has had exactly one caller shape: a human at a terminal, standing in a repo, running
`tp`. Command-centre is a second caller of a different shape — one long-lived Go process with a
ticker, four concurrent agent slots, managing two repos it does not stand in.

Its specification originally forbade importing treepad at all
(`docs/command-centre-v1.md` decision 5: the surface is `tp new --base` and `tp remove --force`,
by subprocess). That reads as a design preference but was not one. No importable surface existed,
so subprocess was the only option available, and the rule recorded the constraint rather than a
judgement. Once the constraint is named as such, the question becomes what an importable surface
should look like — not whether to have one.

The answer is less work than it appears, because the CLI layer holds no logic worth extracting.
`lifecycle.New` and `lifecycle.Remove` already take a context, a dependency bundle and an input
struct; `internal/commands/new.go` is 84 lines of flag wiring over them and `remove.go` is 26.
Go's `internal/` rule governs import visibility, not linking, so a package at the module root can
reach every internal package without moving any of them. What is genuinely at stake is therefore
not structure but *commitment*: which identifiers become public, and what they are promised to
mean.

Three things made that commitment harder than simply re-exporting what exists.

**The dependency bundle is not publishable.** `deps.Deps` names eight interfaces across eight
packages plus a concrete `*ui.Printer`. It is the right shape for a composition root and the
wrong shape for an API — exporting it would freeze eight interfaces to buy an injection seam
nothing has asked for.

**The two callers want opposite things.** The CLI needs `--open`, `--current`, `--ticket` and the
`__TREEPAD_CD__` fd-3 emit. Every one of those is a terminal affordance that a library caller must
carry as dead weight. `lifecycle.New` also returns the *main* worktree path and discards the
created path it just computed — correct for a caller about to `cd`, useless for a caller that
wants to know what it made.

**Treepad's notion of "the repo" was ambient.** `worktree.CommandRunner.Run` takes no working
directory and `ExecRunner` never sets `cmd.Dir`, so every git command inherits the process cwd.
That is invisible when the caller is a person standing somewhere. It is disqualifying for a
concurrent process serving two repos, and `os.Chdir` is not available as a workaround because it
is process-global.

## Decision

Treepad exposes a facade package at the module root — `import "github.com/O-Marsters-1997/treepad"`
— presenting `New` and `Remove` over option and result structs, plus sentinel errors. It is a
**sibling** of the CLI, not its replacement: both adapt over `lifecycle.CreateWorktreeWithSync`,
which was already the shared core.

`deps.Deps` and the eight packages it names stay under `internal/`. The facade constructs
`DefaultDeps` itself. Consumers get no injection seam, because adding one later — a `Git` field
on the option structs, nil meaning real git — is a backward-compatible change that does not need
deciding now.

The surface is `New` and `Remove` and nothing else. Callers enumerate worktrees through
`git worktree list --porcelain`, whose format is a more stable contract than a `v0` API, and
liveness stays with the caller rather than becoming a second reading of the Activity file.

Ambient repo selection ends. `ExecRunner` gains a `Dir` field, where the empty string means
inherit — so the CLI's behaviour is unchanged and the facade names the repo it means. One field
covers git, non-interactive hooks and the `open` command, because `DefaultDeps` already threads a
single runner into all three.

Library cuts are otherwise identical to `tp new` — config sync runs, lifecycle hooks run — so a
worktree cut by a program is indistinguishable from one cut by hand. There are exactly two
deliberate divergences, both narrowing what the library will do:

- **Interactive hooks are refused.** `passthrough.OSRunner` opens `/dev/tty` when stdio is not a
  terminal, so an `interactive = true` hook fired from a background tick would seize the operator's
  terminal and block on input nobody is there to give.
- **`Force` never wipes a dirty worktree.** It means `git branch -D` in place of `-d`, which is what
  a squash-merged ticket requires and all a program legitimately needs. Destroying uncommitted work
  is a decision for a human who can see it.

Both divergences are refusals, and both are visible in the public error set as
`ErrInteractiveHook` and `ErrDirty`. The library is permitted to do less than the CLI; it is never
permitted to do something different.

The boundary is held by a golden `go doc -all` snapshot rather than by `batch/api_test.go`'s
no-internal-imports rule, which the facade cannot satisfy by design — importing internals is the
whole mechanism. The snapshot catches what matters instead: an exported signature naming an
internal type (which Go permits silently — consumers can hold such a value but never name it) and
unplanned growth of the surface itself.

## Consequences

Command-centre stops needing `tp` installed, on `PATH`, and version-matched; treepad becomes a
`go.mod` requirement compiled into its binary. It loses a subprocess seam that was load-bearing
for testing — `e2e/faketp` substituted a fake `tp` binary via `PATH` — and gains e2e tests that run
against a real `.treepad.toml`. More faithful, slower, not stubbable.

Command-centre's decision 3, "never delete a worktree that is dirty", becomes structurally true
rather than a rule the caller must remember on every tick. Only the dirty half: treepad does not
look at unpushed commits, so that check stays with the caller.

Two adapters over one core can drift. The exposure is small — `lifecycle.New` is 47 lines and does
almost nothing the core does not — but it is real, and a change to cut semantics must now be made
in a place both adapters see rather than in either adapter.

The facade is the first thing in this repo that cannot be refactored freely. At `v0` the module
protocol permits breaking it, and the golden snapshot makes surface changes visible rather than
impossible, so the practical discipline is narrow: do not ship a field whose meaning is still
uncertain. Almost everything deferred here — sentinels, option fields, result fields, further
functions — is additive when it is wanted.

Publishing a Go API also requires `v`-prefixed tags, which this repo has never cut. Every existing
tag is `0.4.9`-shaped, so `go get` resolves a commit pseudo-version rather than any release.

## Alternatives considered

**Export `deps` and the eight packages it names.** Full injection, consumers can fake anything.
Rejected: nine published packages with every interface signature frozen, to serve one consumer
that wants two functions — the opposite of the discipline `batch/api_test.go` exists to enforce,
and reversible only by breaking everyone.

**Make the facade the single path and have the CLI call it.** One code path, zero drift by
construction. Rejected: the facade's option struct then grows `Open`, `Current`, `OutputDir` and a
cd sink to serve the terminal, so four of seven public fields would exist for a caller that is not
the audience. Drift is a smaller cost than a public API shaped by the wrong consumer.

**Wrap `lifecycle.New` rather than the core.** Least new code inside treepad. Rejected: it emits
the cd directive that must then be suppressed, and returns the main worktree path, so the facade
would have to re-derive the created path — duplicating the slug formula that command-centre's
decision 5 was specifically written to avoid duplicating.

**Thread a directory through `CommandRunner.Run`.** The architecturally honest fix, matching
`passthrough.Runner` which already takes one. Rejected as disproportionate: 34 call sites across
seven non-test files, five identical interface declarations and several test fakes — and one of
those declarations is `batch.Runner`, published API, broken to repair internal plumbing. A struct
field on `ExecRunner` achieves the same result additively.

**Inject `git -C <dir>` from the facade.** Touches no existing code. Rejected: it fixes git only.
Non-interactive hooks and the `open` command run through the same runner and would still execute in
the caller's working directory — shipping a known hole of exactly the shape the interactive-hook
refusal exists to close.

**Keep subprocess only.** Rejected: it was never chosen on merit. It was the only option, and the
rule that recorded it said so once the constraint was examined.
