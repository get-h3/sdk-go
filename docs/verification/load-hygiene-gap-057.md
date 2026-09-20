# GAP-057 — count-guard load hygiene (sdk-go arm)

Date: 2026-09-20
Repo: get-h3/sdk-go
Subject: `scripts/check-test-count.sh`
Result: output byte-identical, process starts 291 → 36 on a direct run

## Why this exists

Fleet load-hygiene directive (2026-09-19): one or two projects must not eat the
whole box. The umbrella arm was fixed at h3 tick #415 (one interpreter per
artifact → one for all). This is the sdk-go arm, and the count guard was the
worst offender in this repo.

It was the worst offender because it pays a OS process boundary per file per
assertion, and `scripts/countguard/guard_test.go` invokes it from 18 Test
functions (17 call sites) through `runGuard()` — so every `go test ./...` in
this repo multiplied the cost by ~17. The entire output of the guard on a clean
tree is four PASS lines.

## Root cause — per-file process fan-out in POSIX sh

Both sweeps walked the tracked file list in shell and started fresh coreutils
for every assertion on every artifact:

| site | shape | cost |
|---|---|---|
| `has_banner()` | `head -n 25 "$f" \| grep -q -E` | 2 processes per scanned file |
| retired-literal sweep | `grep -n -I -E` per file, then `grep -v`, `sed`, `tr`, `wc` per file with hits | 1–5 per file |
| canonical-claim sweep | one `awk` process per file | 1 per file |
| dated-record loop | `grep -q -I -E` + `head`+`grep` per dated file | 3 per dated file |
| static suite fallback | `grep -c -E '^func Test'` per tracked test file | 1 per test file |

That is roughly eight process starts on each of the ~59 scanned artifacts — to
classify a few hundred bytes of text.

## What was batched

1. **The banner predicate and both per-file tables now run in ONE `awk`
   process** which reads every scanned file as an argument. A file's lines are
   buffered so its retired-literal hits print before its claim hits — the same
   report order the per-file loop produced. The banner test is `FNR <= 25`,
   the same first-25-lines window `head -n 25` used, evaluated in the same pass
   that classifies the file.
2. **The dated-record loop is one `awk`** over the dated files that prints the
   names quoting a retired literal with no point-in-time banner; the shell only
   formats the FAIL lines.
3. **The static suite-count fallback is one `awk`** over the tracked test files
   instead of a `grep -c` per file.
4. Repeated `wc -l | tr -d ' '` pairs collapsed to one per report block.
5. The two EREs reach awk through the environment (`ENVIRON`), not `-v`,
   because `-v` escape-processes backslashes and these are EREs.

Nothing was skipped, cached, time-boxed or short-circuited: `is_scanned`,
`is_excluded`, the first-25-lines banner window, the inline
`count-ok-historical` marker, the "not reported twice" rule, the dated-record
requirement, all four `H3_SDK_*` overrides and the 0/1/2 exit contract are
unchanged.

## Before / after

`/proc/loadavg` at measurement time: 7.5–10.5 (shared fleet box, other projects
active). Counts are `execve` lines from
`strace -f -e trace=execve -o trace.txt sh scripts/check-test-count.sh`:

| measurement | before | after |
|---|---|---|
| execve lines, direct run (3 runs) | 291 / 291 / 291 | 36 / 36 / 36 |
| execve lines, `go test ./scripts/countguard/ -count=1` (guard invoked ~17x) | 1166 | 366 |
| wall clock, direct run, 3 runs | 1.00 / 0.97 / 0.89 s | 0.40 / 0.43 / 0.40 s |
| maxrss, direct run | ~148 MB | ~147 MB |
| countguard suite wall | 14.0 s | 12.4 s |

The direct-run "before" number was 287 while this record was still untracked and
291 once it was tracked. That +4 is the per-file cost model made visible in a
single step: one added scanned Markdown file cost `head` + `grep` (banner) +
`grep` (retired sweep) + `awk` (claim sweep) = four process starts. After the
change the same added file costs zero.

Top spawned binaries, direct run (`execve("...")` basenames, count desc):

```
before: grep 136, head 67, awk 60, link 7, tr 2, sed 2, git 2, wc 1, ...
after:  link 7, grep 4, tr 2, sed 2, head 2, git 2, awk 2, wc 1, ...
```

grep + head + awk go from 263 to 8 combined. What remains is dominated by the
guard's actual subject — the live suite derivation (`go test ./... -list '^Test'`:
go, compile, link, one test binary per package, vet, gcc) — plus the two awk
passes, the canonical-file reads (grep/sed/head), the whitespace check and the
file-list derivation.

## Byte-identity proof

Primary gate: stdout + stderr + exit code, original vs fixed.

```
cp /tmp/gap057-orig.sh scripts/.gap057-orig.sh          # untracked -> not in `git ls-files`
sh scripts/.gap057-orig.sh      >/tmp/before.sh.out 2>&1; echo $? >/tmp/before.sh.code
sh scripts/check-test-count.sh  >/tmp/after.sh.out  2>&1; echo $? >/tmp/after.sh.code
diff /tmp/before.sh.out /tmp/after.sh.out               # empty
diff /tmp/before.sh.code /tmp/after.sh.code             # empty
```

Both diffs are empty (exit 0). Repeated with `bash` in place of `sh`: empty.

A clean tree only exercises the empty-report path, so the same differential was
run over 14 scenarios against identical trees and environments — original and
fixed, stdout+stderr+exit code diffed — with **0 differences**:

```
IDENTICAL  treeA-rich          (15 lines incl. code)   6 real hits
IDENTICAL  treeB-dated-only    (10 lines incl. code)   2 dated records flagged
IDENTICAL  treeC-clean         (6 lines incl. code)
IDENTICAL  treeD-whitespace    (5 lines incl. code)    exit 2
IDENTICAL  treeE-no-suite      (4 lines incl. code)    exit 2
IDENTICAL  treeF-suite-drift   (5 lines incl. code)    exit 1
IDENTICAL  treeG-battery-drift (6 lines incl. code)    exit 1
IDENTICAL  treeH-shim-not-number (5 lines incl. code)  exit 2
IDENTICAL  treeI-canon-missing (3 lines incl. code)    exit 2
IDENTICAL  treeJ-canon-malformed (3 lines incl. code)  exit 2
IDENTICAL  treeK-canon-dupe    (3 lines incl. code)    exit 2
IDENTICAL  treeL-battery-missing (3 lines incl. code)  exit 2
IDENTICAL  repo-live           (5 lines incl. code)    live derivation
IDENTICAL  repo-static         (5 lines incl. code)    static derivation (H3_SDK_LIVE=0)
```

## Negative control — it still fails, not just faster

`treeA-rich` is a scratch `H3_SDK_SCAN_ROOT` built so that its report is **not**
empty: a dozen files, six deliberately offending lines across four of them.
Both versions produced identical stdout+stderr, the same six hits in the same
order, and exit status 1:

| file:line | what the offending line carries | reported as |
|---|---|---|
| `README.md:1` | a retired battery fraction, plus the word PASSED | retired-literal hit |
| `README.md:2` | a stale suite claim (not the canonical suite size) | claim hit |
| `late-banner.md:32` | a retired battery fraction (banner past line 25) | retired-literal hit |
| `mixed.md:1` | two retired forms + a fraction + a stale suite claim | retired-literal hit only |
| `docs/guide.md:1` | two retired tokens on one line | one retired-literal hit |
| `docs/guide.md:3` | a stale suite claim | claim hit |

The fixture's own counts are deliberately described rather than written here:
this record is itself a tracked current-state surface and the guard sweeps it,
so quoting the fixtures verbatim would fail the very check it documents (see
Limits below). The committed reproduction of these branches is
`scripts/countguard/guard_test.go`, which assembles the retired digits at
runtime for the same reason.

That one tree exercises every branch of the batched pass in a single run:

* a retired literal and a claim on the **same line** (`README.md:1`,
  `mixed.md:1`, `late-banner.md:32`) are reported **once** — the retired sweep
  owns the line and the claim sweep suppresses it, exactly as before;
* a claim on a **different line** is reported separately (`README.md:2`,
  `docs/guide.md:3`), and two retired tokens on one line count as one hit
  (`docs/guide.md:1`) — the buffer is per file, so this ordering is preserved
  rather than merged into one global pass;
* a **banner inside the first 25 lines** (`bannered.md`) drops the whole file;
  a banner **past line 25** (`late-banner.md`) does not;
* the `count-ok-historical` marker still exempts a line (`narration.md`);
* canonical totals are still not claimed (`canonical.md`: the canonical battery
  and suite numbers, and a fraction whose total sits below the reporting floor);
* excluded surfaces are still excluded (`CHANGELOG.md`, `dist/**`);
* dated records carrying a banner, and dated records with no retired count,
  still pass.

`treeB-dated-only` covers check (f) — it reaches the dated loop only because
HITS is zero — and both versions named the same two records in the same order,
`docs/audit-2026-02/y.md` then `docs/dogfood/2026-01-03-bad.md`, each with the
`no point-in-time banner` message.

`treeD-whitespace`, `treeE-no-suite`, `treeF-suite-drift`, `treeG-battery-drift`,
`treeH-shim-not-number` and `treeI/J/K/L` hold the exit-2 and exit-1 contracts
(canonical file missing / malformed / duplicated / battery line missing, an
unreadable suite source, a non-numeric sibling count, suite drift, battery
drift).

## Regression

```
go test ./scripts/countguard/... -count=1 -v   -> 18/18 PASS, 0 FAIL (plus 4 subtests)
go test ./... -count=1 -short                  -> exit 0, all six test packages ok
go vet ./...                                   -> exit 0
gofmt -l .                                     -> empty
go build ./...                                 -> exit 0
sh scripts/check-test-count.sh                 -> exit 0, PASS battery=46 suite=160
```

## Limits of the measurement

* Spawn counts vary a few lines run to run (the live-derivation child processes
  are not deterministic); the 287 → 36 gap is far outside that jitter, and the
  per-binary table is stable.
* Measurements were taken on a shared, loaded box (loadavg 7.5–10.5). The wall
  clock is therefore an upper bound on the guard's own cost and moved with the
  box; the execve counts do not depend on load.
* `strace -f` counts `execve` calls, i.e. process starts. It does not measure
  CPU time, page faults or scheduler cost — wall clock and maxrss are reported
  separately for that reason. maxrss is dominated by the `go test` child and is
  effectively unchanged.
* Byte-identity was established on this host with GNU coreutils and
  mawk 1.3.4. The batched pass uses only POSIX awk (ENVIRON, dynamic regex
  `match`, `delete arr[i]`, `[[:space:]]`, local function parameters), but a
  different awk implementation is the one thing this evidence cannot speak for.
* One deliberate, unreachable-in-this-repo behaviour difference: the old
  per-file `grep -n -I` skipped binary files silently, while `awk` scans any
  file it is given as text. No file in the scanned set is binary (verified with
  `grep -qI` over all of them), and a read failure now exits non-zero rather
  than being swallowed — loud beats a silently truncated scan.
* `python3` is still not required and still not used: the guard remains POSIX
  sh + coreutils (git, grep, sed, awk, wc).
* This record is itself a tracked current-state surface, so this guard sweeps
  it: a quoted retired count in a count-shaped form would fail the very check
  the file documents. The negative-control fixture is therefore described by
  shape rather than quoted verbatim, and the offending literals in the tables
  above appear only as prose. `scripts/countguard/guard_test.go` assembles the
  retired digits at runtime for exactly the same reason.
