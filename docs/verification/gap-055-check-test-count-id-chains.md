# SDKGO-GAP-055 — identifier chains are not counts

**Ticket:** SDKGO-GAP-055 (P3) — `scripts/check-test-count.sh` false-positived on
GAP-id chains, so a doc that never quoted a battery count failed the gate.
**Branch:** `wt/SDKGO-GAP-055`.
**Files:** `scripts/check-test-count.sh` (fix), `scripts/countguard/guard_test.go`
(tests), `scripts/test-count.txt` (suite re-pin), this record.

> Verbatim probe/output lines below carry the guard's `count-ok-historical`
> marker where the numbers quoted belong to the probe's own synthetic root or to
> the pre-fix tree rather than to this one — this document is a living record, so
> its era-correct numbers must say so.

---

## 1. Symptom

CI job `Build & Test (1.26)`, step `Compliance-test count guard`, failed on
`6132b4a` while every test passed:

    FAIL: docs/dogfood/2026-09-18-integration.md quotes a retired count with no point-in-time banner.

That record quotes no battery count. Its only count-shaped text is an id chain:

    - Evidence that the 09-01 dogfood P2s (5 rows) and GAP-041/042/045/046 remain

## 2. Root cause

Every count pattern ran against the **raw** line, so the digits of an identifier
were read as counts. Three independent over-matches, all reproduced:

| check | pattern | input | pre-fix report |
|---|---|---|---|
| (d) retired literal | `4[345]/[0-9]+` | `GAP-041/042/045/046` — the `045` is read as the numerator 45, then `/046` as its denominator | `FAIL: … quotes a retired count with no point-in-time banner.` |
| (e) whole-suite total | `[0-9]+/[0-9]+` | `SDKGO-GAP-058/059/060` on any line that also says "tests" | `total claim 058/059 is not a canonical total` `count-ok-historical` |
| (e) suite claim | `[0-9][0-9][0-9][- ]tests?` | `GAP-058 tests the atomic claim path.` | `suite claim 58 tests != 168` |

Chains carrying a retired-id fragment (the tail of `GAP-041/042/045/046`), three-digit ids in front of
the word "tests", and id chains in prose were all read as quotes. Plural ids in
this repo are written exactly that way (`GAP-041/042/045/046`,
`GAP-008/009/GAP-DOG-001..003`, `DF-H3-SDK-GO-FOREMAN-8`), so the class was
guaranteed to re-fire.

## 3. Fix

`scripts/check-test-count.sh` now strips identifier tokens from each line before
any count pattern runs (`scrub_ids`), in all three places that consume them —
check (d), check (e) and check (f):

* `ID_TOKEN` = `[A-Z][A-Za-z0-9]*(-[A-Za-z0-9]+)*-[0-9]+(/[0-9]+)*` — an
  upper-case id shaped `NAME[-SEGMENT…]-<digits>` with an optional `/NNN` chain:
  `GAP-041`, `GAP-041/042/045/046`, `SDKGO-GAP-055`, `DF-H3-SDK-GO-FOREMAN-8`.
  Upper-case-first deliberately: `GAP-045` is an identifier, `battery-45` is
  prose and stays subject to every pattern.
* `ID_CHAIN` = `[0-9]+/[0-9]+/[0-9]+(/[0-9]+)*` — a digits/slash run with more
  than one slash. A count quote is only ever `N/N`.

Reported hits keep quoting the **original** line, never the scrubbed copy.
Checks (a), (b) and (c) — canonical parse, live suite parity, shim battery
parity — are untouched, and check (d) stays context-free: a retired fraction
needs no surrounding prose to fail. Nothing was exempted except identifier
tokens.

## 4. Evidence

### 4.1 The real CI-red input (pristine `6132b4a` tree, its own canonical file)

Pre-fix script, pre-fix tree:

    $ sh scripts/check-test-count.sh                                     # 6132b4a   count-ok-historical
    check-test-count: suite agrees (130 tests via live (`go test ./... -list '^Test'`))   count-ok-historical
    check-test-count: no stale count literals in current-state surfaces
    FAIL: docs/dogfood/2026-09-18-integration.md quotes a retired count with no point-in-time banner.
    EXIT=1

Same tree with the chain neutralised in that one line (`GAP-041/042/045/046` →
`GAP-(ids)`) — the chain is the **only** trigger:

    EXIT=0

Fixed script, same tree, same input:

    $ H3_SDK_SCAN_ROOT=<6132b4a tree> H3_SDK_COUNT_FILE=<…>/scripts/test-count.txt sh scripts/check-test-count.sh
    check-test-count: suite agrees (130 tests via live (`go test ./... -list '^Test'`))   count-ok-historical
    check-test-count: no stale count literals in current-state surfaces
    check-test-count: PASS — canonical battery=46, suite=130; battery parity NOT VERIFIED (no shim count at …/absent.txt); current-state prose agrees   count-ok-historical
    EXIT=0

### 4.2 Synthetic matrix (scratch root, canonical `battery=46 suite=100`)

Pre-fix = pristine `6132b4a` script; post-fix = this branch. Every input below
quotes no battery count.

| case | input | pre-fix | post-fix |
|---|---|---|---|
| FP-A | `- wave h3: SDKGO-GAP-058/059/060 landed; tests pass, battery 46/46` | exit 1 — `total claim 058/059 is not a canonical total` | exit 0 | `count-ok-historical`
| FP-B | `- GAP-058 tests the atomic claim path.` | exit 1 — `suite claim 58 tests != 100` | exit 0 |
| FP-C | `- Evidence that the P2 rows GAP-041/042/045/046 remain open.` (bannerless dated record) | exit 1 — banner demanded | exit 0 |
| FP-D | `- 5 rows closed (GAP-048/049/050) tests and docs untouched.` | exit 1 — `total claim 048/049 …` | exit 0 | `count-ok-historical`
| NEG-1 | `the battery reports 45/45 PASSED` `count-ok-historical` | exit 1 | **exit 1** |
| NEG-2 | `\| 45/45 \|` (no countable word anywhere on the line) `count-ok-historical` | exit 1 | **exit 1** |
| NEG-3 | `- GAP-048/049/050 battery 45/45 PASSED` `count-ok-historical` | exit 1 | **exit 1** |
| NEG-4 | `Run the suite: 101 tests green.` `count-ok-historical` | exit 1 | **exit 1** |
| NEG-5 | `the battery scored 45/45` in a bannerless dated record `count-ok-historical` | exit 1 | **exit 1** |
| OK-1 | `Battery 46/46 PASSED; suite 100 tests green.` `count-ok-historical` | exit 0 | exit 0 |
| OK-2 | same as NEG-5 but with the point-in-time banner | exit 0 | exit 0 |

FP-A/FB-B also show the two `(e)` mechanisms; FP-C is the CI failure itself;
NEG-1..5 are the strength controls that must keep failing (and do).

### 4.3 In-repo regression tests

`scripts/countguard/guard_test.go` gained three top-level tests (subtests for the
strength matrix). Against the **pre-fix** guard:

    --- FAIL: TestGuardIgnoresIDChainsInProse (0.06s)
    --- FAIL: TestGuardDoesNotDemandABannerForAnIDChain (0.05s)
    --- PASS: TestGuardStillFlagsRetiredCountsBesideAnIDChain (0.18s)

Against the fixed guard:

    --- PASS: TestGuardIgnoresIDChainsInProse (0.06s)
    --- PASS: TestGuardDoesNotDemandABannerForAnIDChain (0.10s)
    --- PASS: TestGuardStillFlagsRetiredCountsBesideAnIDChain (0.15s)
        --- PASS: TestGuardStillFlagsRetiredCountsBesideAnIDChain/beside_an_id_chain (0.05s)
        --- PASS: TestGuardStillFlagsRetiredCountsBesideAnIDChain/alone_in_a_table_cell (0.05s)
        --- PASS: TestGuardStillFlagsRetiredCountsBesideAnIDChain/in_a_dated_record_on_the_chain_line (0.05s)

`TestGuardExempts*` / `TestGuardRequiresABannerOnDatedRecords` and the other
`countguard` cases are unchanged and still green — the exemption is the id token
alone, not the checks around it.

## 5. Gate battery

    go build ./...                     # OK
    go vet ./...                       # OK
    gofmt -l .                          # empty
    go test -race -count=1 ./...        # ok — 4 packages with tests, 168 tests
    sh scripts/check-test-count.sh      # PASS — battery=46, suite=168

Suite math: 165 at the branch point + 3 new top-level tests here = 168, which is
why `scripts/test-count.txt` and the suite-size line in
`docs/verification/gap-058-059-060-claim-turns-body-cap.md` moved in this same
commit (the guard's own prescription, check (e)).

## 6. Scope note

The fix removes *identifier tokens* and multi-slash digits/slash runs from the
count-pattern input. It does not relax: the canonical-count parse (a), live suite
parity (b), shim battery parity (c), the retired-literal set (d), the whole-suite
total rule (e), or the dated-record banner rule (f). A document that quotes a
retired total still fails — with or without an id chain beside it — and a dated
record that quotes one still has to declare itself historical.
