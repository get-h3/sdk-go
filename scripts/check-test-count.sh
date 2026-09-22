#!/bin/sh
# check-test-count.sh — the Go SDK polices its OWN count prose (H3-GAP-087).
#
# Why this exists: the compliance battery in get-h3/shim is the single source of
# truth for how many compliance tests exist, and this repo's own suite has a
# count of its own — but nothing in THIS repo checked the prose around either.
# The stale-count class re-offended here repeatedly (CI pins that hardcoded
# "45/45" and red-lined main while every test passed, README/integration-guide/
# SKILL/CONTRIBUTING left at 43/44/45 after the battery reached 46) because the
# only sweep lived in the umbrella repo, where an SDK-only doc edit is never
# seen. The Go SDK owns its truth now.
#
# Canonical inputs — scripts/test-count.txt:
#   battery=46   the get-h3/shim compliance battery (`h3-test`): every "N/N
#                compliant" / "N tests, 6 categories" claim in this repo is
#                about this number.
#   suite=110    this repo's own Go test suite: `go test ./... -list '^Test'`.
#
# Checks:
#   a. canonical parse       — scripts/test-count.txt must exist and hold exactly
#                              one `battery=` and one `suite=` bare number, else
#                              exit 2 (guard misconfigured).
#   b. suite parity          — the LIVE count (`go test ./... -list '^Test'`,
#                              falling back to a static `^func Test` scan of the
#                              tracked test files when the Go toolchain is not
#                              available) must equal `suite=`, else exit 1. This
#                              is what catches the suite growing before any doc
#                              notices.
#   c. battery parity        — when a sibling shim checkout is present its
#                              scripts/test-count.txt must agree with `battery=`,
#                              else exit 1 (the battery moved and this repo's
#                              prose is now stale). No sibling checkout → a loud
#                              "battery parity NOT VERIFIED" caveat and exit 0:
#                              a fresh clone or a CI checkout has no ../shim and
#                              this repo must not depend on one — but the pass
#                              must be impossible to mistake for a checked one.
#                              Set H3_SDK_REQUIRE_SHIM_PARITY=1 to make that
#                              absence a misconfiguration (exit 2) instead: the
#                              umbrella checkout and monorepo users can then
#                              fail fast.
#   d. retired-literal sweep — no tracked current-state surface may still
#                              advertise a RETIRED battery count (43/44/45 in
#                              count-shaped forms, plus any "N/44"-style fraction
#                              against a retired total).
#   e. canonical-claim sweep — a living doc that states a suite size
#                              ("<NNN> tests") or a whole-suite total
#                              ("<NN>/<NN>", 40+) must state the canonical
#                              number. This catches a stale suite literal
#                              ("144 tests") that was never a battery count.
#   f. dated-report banner   — a dated record (docs/dogfood/YYYY-MM-DD-*,
#                              docs/audit-YYYY-MM-*) that quotes a retired count
#                              must open with a point-in-time banner
#                              "> **Historical (YYYY-MM-DD):** ...", which is
#                              what makes its exemption unambiguous to a reader.
#
# Identifier tokens are never counts (SDKGO-GAP-055). Before any count pattern
# runs, each line is stripped of id tokens — `GAP-041`, `GAP-041/042/045/046`,
# `SDKGO-GAP-055`, `DF-H3-SDK-GO-FOREMAN-8` — and of any digits/slash run with
# more than one slash (a count quote is always N/N). Without that stripping the
# retired-fraction alternative in (d) read the "045" inside `GAP-041/042/045/046`
# as the retired total 45 and red-lined main on a doc that quoted no count at
# all; the same over-match made `GAP-058/059` a "total claim 058/059" in (e) and
# `GAP-058 tests` a "suite claim 58 tests". The exemption is the id token alone:
# a real count beside one (`GAP-048/049/050 battery 44/45 PASSED`) still fails,
# and a retired fraction with no surrounding prose at all (`| 43/44 |`) still
# fails, because (d) remains context-free.
#
# Historical exemptions (deliberate, narrow):
#   * CHANGELOG.md, e2e-output/, dist/, .coding-hermes/, .gitreins/, .vfs/ —
#     release/board/state records, never rewritten.
#   * dated reports (docs/dogfood/YYYY-MM-DD-*, docs/audit-YYYY-MM-*) — carry the
#     count that was true when written; check (f) requires them to say so.
#   * any single line carrying the inline marker `count-ok-historical` — an
#     era-correct number quoted inside an otherwise-living document.
#   * any file whose head (first 25 lines) carries the same point-in-time
#     banner — a document that declares itself a historical record.
#
# Exit codes: 0 = pass — with a loud "battery parity NOT VERIFIED" caveat on
#             every summary line when the sibling shim count is absent;
#             1 = drift; 2 = guard misconfigured (missing/bad inputs, or
#             H3_SDK_REQUIRE_SHIM_PARITY=1 set while the sibling count is
#             absent/unreadable).
# Dependencies: POSIX sh + coreutils (git, grep, sed, awk, wc). No venv, no
# network. The Go toolchain is optional (used for the live count only).
#
# Overrides (used by scripts/countguard/guard_test.go to drive every branch
# hermetically): H3_SDK_COUNT_FILE, H3_SDK_SCAN_ROOT, H3_SDK_SHIM_COUNT_FILE,
# H3_SDK_LIVE (1 = prefer the live Go count, 0 = force the static scan),
# H3_SDK_REQUIRE_SHIM_PARITY (1 = a missing sibling shim count is a
# misconfiguration: exit 2 instead of the pass-with-caveat; unset or 0 = the
# CI-safe default that still exits 0 and says loudly that parity was NOT
# verified).

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=${H3_SDK_SCAN_ROOT:-$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)}

# ---- (a) canonical counts --------------------------------------------------
CANON_FILE=${H3_SDK_COUNT_FILE:-$SCRIPT_DIR/test-count.txt}
if [ ! -f "$CANON_FILE" ]; then
    echo "FAIL: canonical count file missing: $CANON_FILE" >&2
    echo "      Fix: create it with the two lines 'battery=<N>' and 'suite=<N>'." >&2
    exit 2
fi

canon_value() {
    # $1 = key. Prints the value only when the file holds EXACTLY one such line.
    n=$(grep -c -E "^$1=[0-9][0-9]*\$" "$CANON_FILE" 2>/dev/null || true)
    if [ "$n" != "1" ]; then
        return 1
    fi
    sed -n "s/^$1=\([0-9][0-9]*\)\$/\1/p" "$CANON_FILE" | head -n 1
}

if ! BATTERY=$(canon_value battery); then
    echo "FAIL: $CANON_FILE must contain exactly one 'battery=<number>' line" >&2
    echo "      (the get-h3/shim compliance battery count)." >&2
    exit 2
fi
if ! SUITE=$(canon_value suite); then
    echo "FAIL: $CANON_FILE must contain exactly one 'suite=<number>' line" >&2
    echo "      (this repo's own test-suite count)." >&2
    exit 2
fi

# ---- (b) suite parity: canonical vs the repo's real suite source -----------
MODE=""
COUNT=""
LIVE=${H3_SDK_LIVE:-1}

if [ "$LIVE" = "1" ] && command -v go >/dev/null 2>&1; then
    LIST=$( cd "$ROOT" && go test ./... -list '^Test' 2>/dev/null | grep -E '^Test' || true )
    if [ -n "$LIST" ]; then
        COUNT=$(printf '%s\n' "$LIST" | wc -l | tr -d ' ')
        MODE="live (\`go test ./... -list '^Test'\`)"
    fi
fi

if [ -z "$COUNT" ]; then
    FILES=$( cd "$ROOT" && git ls-files -- '*_test.go' 2>/dev/null || true )
    if [ -z "$FILES" ]; then
        # Not a git checkout (or no tracked test files): fall back to a walk, so
        # the guard still works on an exported tree or a test scratch root.
        FILES=$( cd "$ROOT" && find . -type f -name '*_test.go' | sed 's|^\./||' || true )
    fi
    if [ -z "$FILES" ]; then
        echo "FAIL: cannot derive the Go suite count — no *_test.go files found" >&2
        echo "      under $ROOT and no usable Go toolchain." >&2
        echo "      Fix: run this guard from the repo (or set H3_SDK_SCAN_ROOT)." >&2
        exit 2
    fi
    # One awk process counts every test file (GAP-057): the per-file `grep -c`
    # cost a process start per file for the same sum.
    COUNT=0
    set --
    for f in $FILES; do
        if [ -f "$ROOT/$f" ]; then
            set -- "$@" "$ROOT/$f"
        fi
    done
    if [ $# -gt 0 ]; then
        COUNT=$(awk '/^func Test/ {n++} END {print n + 0}' "$@")
    fi
    MODE="static (tracked *_test.go, \`^func Test\`)"
fi

if [ "$COUNT" = "0" ]; then
    echo "FAIL: derived Go suite count is 0 — the test files moved or are unreadable." >&2
    echo "      Fix: update the derivation in this guard." >&2
    exit 2
fi

if [ "$COUNT" != "$SUITE" ]; then
    echo "FAIL: suite drift — $MODE reports $COUNT tests" >&2
    echo "      but $CANON_FILE says suite=$SUITE." >&2
    echo "      Fix: decide which is truth, then update scripts/test-count.txt AND" >&2
    echo "      every prose line that states a suite size." >&2
    exit 1
fi
echo "check-test-count: suite agrees ($COUNT tests via $MODE)"

# ---- (c) battery parity against the sibling shim's canonical count --------
SHIM_CANON=${H3_SDK_SHIM_COUNT_FILE:-$ROOT/../shim/scripts/test-count.txt}
PARITY_VERIFIED=0
if [ -f "$SHIM_CANON" ]; then
    SHIM_BATTERY=$(tr -d ' \t\r\n' < "$SHIM_CANON")
    case "$SHIM_BATTERY" in
        '' | *[!0-9]*)
            echo "FAIL: $SHIM_CANON is not a bare number: '$SHIM_BATTERY'" >&2
            echo "      Fix: point H3_SDK_SHIM_COUNT_FILE at the shim's canonical" >&2
            echo "      count file, or unset it to skip battery parity." >&2
            exit 2
            ;;
    esac
    if [ "$SHIM_BATTERY" != "$BATTERY" ]; then
        echo "FAIL: battery drift — this repo pins battery=$BATTERY" >&2
        echo "      but the shim's canonical battery count is $SHIM_BATTERY." >&2
        echo "      Fix: the battery moved. Update scripts/test-count.txt to" >&2
        echo "      battery=$SHIM_BATTERY and sweep every prose count the guard names." >&2
        exit 1
    fi
    PARITY_VERIFIED=1
    echo "check-test-count: battery agrees with the shim ($BATTERY tests)"
else
    # GAP-053: a fresh clone or a CI checkout has no ../shim, so the pass must
    # stay (an unconditional exit here would red-line every CI run) — but it
    # must be impossible to read as "parity checked and agreed".
    echo "check-test-count: ============================================================"
    echo "check-test-count: WARNING — battery parity NOT VERIFIED (GAP-053): no shim"
    echo "check-test-count: canonical count at $SHIM_CANON — the battery=$BATTERY"
    echo "check-test-count: claim was checked against this repo only, not against"
    echo "check-test-count: the shim's live battery count."
    echo "check-test-count: set H3_SDK_REQUIRE_SHIM_PARITY=1 to fail (exit 2) instead."
    echo "check-test-count: ============================================================"
    if [ "${H3_SDK_REQUIRE_SHIM_PARITY:-0}" = "1" ]; then
        echo "FAIL: shim battery parity not verifiable: no count file at $SHIM_CANON" >&2
        echo "      H3_SDK_REQUIRE_SHIM_PARITY=1 demands the sibling shim count," >&2
        echo "      so an absent/unreadable file is a misconfiguration (exit 2)." >&2
        echo "      Fix: run from the umbrella checkout (../shim present) or point" >&2
        echo "      H3_SDK_SHIM_COUNT_FILE at the shim's scripts/test-count.txt." >&2
        exit 2
    fi
fi

# ---- (d)+(e) sweeps over tracked current-state surfaces --------------------
# Retired battery totals only — never a bare number, which would also match
# ports, dates and durations.
RETIRED='4[345]-tests?|4[345] tests?|4[345] compliance|4[345] passed|4[345]-test |out of 4[345]|[0-9]+/4[345]|4[345]/[0-9]+'

# SDKGO-GAP-055 — what an identifier token looks like, and what it is not.
# ID_TOKEN: an upper-case id shaped `NAME[-SEGMENT...]-<digits>`, optionally
#   carrying an /NNN id chain (`GAP-041/042/045/046`, `SDKGO-GAP-055`,
#   `DF-H3-SDK-GO-FOREMAN-8`). Upper-case-first on purpose: `GAP-045` is an id,
#   `battery-45` is prose and stays subject to every count pattern.
# ID_CHAIN: a digits/slash run with more than one slash — an id or path chain,
#   never a count quote (counts are only ever N/N).
# Both reach awk through the environment (dynamic regexes in gsub) so no
# backslash is escape-processed twice; see the RETIRED note below.
ID_TOKEN='[A-Z][A-Za-z0-9]*(-[A-Za-z0-9]+)*-[0-9]+(/[0-9]+)*'
ID_CHAIN='[0-9]+/[0-9]+/[0-9]+(/[0-9]+)*'

BANNER_PATTERN='^[[:space:]]*>[[:space:]]*\*\*Historical \([0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]\):'

is_scanned() {
    case "$1" in
        *.md | *.go | *.yml | *.yaml | *.json) return 0 ;;
        Makefile | */Makefile) return 0 ;;
        *) return 1 ;;
    esac
}

is_excluded() {
    case "$1" in
        CHANGELOG.md) return 0 ;;
        e2e-output/* | dist/*) return 0 ;;
        .coding-hermes/* | .gitreins/* | .vfs/*) return 0 ;;
        docs/dogfood/[0-9][0-9][0-9][0-9]-*) return 0 ;;
        docs/audit-[0-9][0-9][0-9][0-9]-*) return 0 ;;
        *) return 1 ;;
    esac
}

# The patterns above are EREs, and `-v` escape-processes backslashes, so they
# reach awk through the environment instead. The banner predicate and both
# per-file sweeps below then run inside ONE awk process (GAP-057): the previous
# shape started fresh coreutils for every assertion on every file
# (head+grep for the banner, grep+sed+tr+wc+awk for the tables) — ~8 process
# starts on each of ~59 artifacts, and the guard is itself invoked ~17x by
# scripts/countguard/guard_test.go.
export RETIRED BANNER_PATTERN ID_TOKEN ID_CHAIN ROOT

if cd "$ROOT" && git rev-parse --git-dir >/dev/null 2>&1; then
    FILES=$(cd "$ROOT" && git ls-files)
else
    FILES=$(cd "$ROOT" && find . -type f | sed 's|^\./||')
fi

# The scans below word-split the file list, so a path with whitespace would be
# scanned as fragments. Fail loudly instead of scanning the wrong thing.
SPACED=$(printf '%s\n' "$FILES" | grep ' ' || true)
if [ -n "$SPACED" ]; then
    echo "FAIL: whitespace in a tracked path breaks this scan: '$SPACED'" >&2
    exit 2
fi

# The scanned set — the same is_scanned / is_excluded / existence filters as
# ever, resolved once here so a single awk process can classify the whole set.
set --
for f in $FILES; do
    if ! is_scanned "$f"; then continue; fi
    if is_excluded "$f"; then continue; fi
    if [ ! -f "$ROOT/$f" ]; then continue; fi
    set -- "$@" "$ROOT/$f"
done

# One pass emits both tables for each file, in the order the per-file loops
# used to: the retired-literal hits first, then the claims not already reported
# on such a line. A file's lines are buffered so its reports print before the
# next file's, keeping the report order byte-identical to the old loop.
DETAIL=""
if [ $# -gt 0 ]; then
    DETAIL=$(awk -v cs="$SUITE" -v cb="$BATTERY" '
        BEGIN {
            pre = ENVIRON["ROOT"] "/"
            prelen = length(pre)
            retired = ENVIRON["RETIRED"]
            banner = ENVIRON["BANNER_PATTERN"]
            idtok = ENVIRON["ID_TOKEN"]
            idchain = ENVIRON["ID_CHAIN"]
            cur = ""
            nlines = 0
            banned = 0
        }
        function rel(p) {
            if (substr(p, 1, prelen) == pre) return substr(p, prelen + 1)
            return p
        }
        # SDKGO-GAP-055 — every count pattern below runs on the line with its
        # identifier tokens (and any multi-slash digits/slash chain) removed, so
        # the "045" of `GAP-041/042/045/046` is never read as a count. Reported
        # lines are the ORIGINAL text, not the scrubbed copy.
        function scrub_ids(s,   t) {
            t = s
            gsub(idtok, " ", t)
            gsub(idchain, " ", t)
            return t
        }
        function flush(   i, line, work, n, tok, parts, b, countable) {
            if (cur == "") return
            if (banned == 0) {
                for (i = 1; i <= nlines; i++) {
                    if (scrub_ids(buf[i]) ~ retired && buf[i] !~ /count-ok-historical/) {
                        printf "%s:%d:%s\n", cur, i, buf[i]
                        seen[i] = 1
                    }
                }
                for (i = 1; i <= nlines; i++) {
                    if (buf[i] ~ /count-ok-historical/) continue
                    work = scrub_ids(buf[i])
                    line = work
                    while (match(line, /[0-9][0-9][0-9][- ]tests?/)) {
                        n = substr(line, RSTART, RLENGTH) + 0
                        if (n != cs && !(i in seen))
                            printf "%s:%d: suite claim %d tests != %d: %s\n", cur, i, n, cs, buf[i]
                        line = substr(line, RSTART + RLENGTH)
                    }
                    line = work
                    countable = (tolower(buf[i]) ~ /tests?|battery|compliance|pytest|vitest|checks?|suite|passed/)
                    while (match(line, /[0-9]+\/[0-9]+/)) {
                        tok = substr(line, RSTART, RLENGTH)
                        split(tok, parts, "/")
                        b = parts[2] + 0
                        if (countable && b >= 40 && b != cb && b != cs && !(i in seen))
                            printf "%s:%d: total claim %s is not a canonical total (%d/%d): %s\n", cur, i, tok, cb, cs, buf[i]
                        line = substr(line, RSTART + RLENGTH)
                    }
                }
            }
            cur = ""
            nlines = 0
            banned = 0
            for (k in seen) delete seen[k]
        }
        FNR == 1 {
            flush()
            cur = rel(FILENAME)
        }
        {
            buf[FNR] = $0
            nlines = FNR
            if (FNR <= 25 && $0 ~ banner) banned = 1
        }
        END { flush() }
    ' "$@")
fi

HITS=0
if [ -n "$DETAIL" ]; then
    printf '%s\n' "$DETAIL"
    HITS=$(printf '%s\n' "$DETAIL" | wc -l | tr -d ' ')
fi

if [ "$HITS" -ne 0 ]; then
    echo "FAIL: $HITS stale count literal(s) above." >&2
    echo "      The battery ships $BATTERY tests and this suite ships $SUITE." >&2
    echo "      Fix: replace each hit with the current number, state the count" >&2
    echo "      count-agnostically, or — for genuinely era-correct narration —" >&2
    echo "      mark the line with the inline marker count-ok-historical." >&2
    exit 1
fi
echo "check-test-count: no stale count literals in current-state surfaces"

# ---- (f) dated records must carry a point-in-time banner -------------------
# Same predicate as the sweep above — a dated record quoting a retired count
# without declaring itself a point-in-time record — but these files are rare,
# so one awk answers which of them fail and the shell only formats the message.
# (The sweep has already exited 1 if it found anything, so this runs on a tree
# that is otherwise clean.)
set --
for f in $FILES; do
    case "$f" in
        docs/dogfood/[0-9][0-9][0-9][0-9]-* | docs/audit-[0-9][0-9][0-9][0-9]-*) ;;
        *) continue ;;
    esac
    if [ ! -f "$ROOT/$f" ]; then continue; fi
    set -- "$@" "$ROOT/$f"
done

UNBANNERED=0
if [ $# -gt 0 ]; then
    UNBANNERED_FILES=$(awk '
        BEGIN {
            retired = ENVIRON["RETIRED"]
            banner = ENVIRON["BANNER_PATTERN"]
            idtok = ENVIRON["ID_TOKEN"]
            idchain = ENVIRON["ID_CHAIN"]
            cur = ""
            quoted = 0
            banned = 0
        }
        # Same id-token stripping as the sweep above (SDKGO-GAP-055): a dated
        # record whose only count-shaped text is an id chain (`GAP-041/042/045/046`
        # read as "45/046") is not a record quoting a retired count — this was
        # the false positive that red-lined CI while every test passed.
        function scrub_ids(s,   t) {
            t = s
            gsub(idtok, " ", t)
            gsub(idchain, " ", t)
            return t
        }
        FNR == 1 {
            if (cur != "" && quoted && banned == 0) print cur
            cur = FILENAME
            quoted = 0
            banned = 0
        }
        {
            if (scrub_ids($0) ~ retired) quoted = 1
            if (FNR <= 25 && $0 ~ banner) banned = 1
        }
        END { if (cur != "" && quoted && banned == 0) print cur }
    ' "$@")
    if [ -n "$UNBANNERED_FILES" ]; then
        printf '%s\n' "$UNBANNERED_FILES" | while IFS= read -r hit; do
            rel=${hit#"$ROOT/"}
            echo "FAIL: $rel quotes a retired count with no point-in-time banner." >&2
        done
        UNBANNERED=$(printf '%s\n' "$UNBANNERED_FILES" | wc -l | tr -d ' ')
    fi
fi
if [ "$UNBANNERED" -ne 0 ]; then
    echo "      Fix: add one line directly under the first heading:" >&2
    echo "        > **Historical (YYYY-MM-DD):** point-in-time record — the counts" >&2
    echo "        > below were correct when written and are not live status." >&2
    exit 1
fi

# ---- (g) PASS summary ------------------------------------------------------
if [ "$PARITY_VERIFIED" = "1" ]; then
    echo "check-test-count: PASS — canonical battery=$BATTERY, suite=$SUITE; current-state prose agrees"
else
    echo "check-test-count: PASS — canonical battery=$BATTERY, suite=$SUITE; battery parity NOT VERIFIED (no shim count at $SHIM_CANON); current-state prose agrees"
fi
exit 0
