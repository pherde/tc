# Task 4 — Comparison of prompt v1 and prompt v2

## Setup

| | |
|---|---|
| CVE | CVE-2023-52835 (perf AUX area bound), backported to `kernel-4.18.0-448.el8` |
| Why this CVE | the upstream patch applies cleanly and builds, but on 4.18 it must use `>= MAX_ORDER`: a clean `git apply` hides a wrong backport |
| Criteria | `criteria.md`, committed before any run (`3bfc733`) |
| Agent | ChatGPT web, the account's default model (the account has no model picker), temporary chats, one turn each, no follow-ups |
| Input | `prompt-v1.md` or `prompt-v2.md`, plus the same `context-bundle.md` (mainline patch and the unpatched target function; the target's `MAX_ORDER` definition was left out on purpose) |
| Runs | v1-a (12:28) and v2-a (12:43), 2026-09-29. Prompt v2 was committed before its run (`082fe0e`) |
| Verification | each diff was tried with `patch -p1 --dry-run` on the baseline -448 file, and the change was compiled in the Task 1 builder container. Every claim was checked against the CNA record and the -448 source |

## Results

| Criterion | v1-a | v2-a |
|---|---|---|
| 1. Upstream fix and dependencies | 2 | 1 (says the fix "landed in the 6.6-era kernel"; it is v6.7) |
| **2. Suitability for 4.18 (key)** | **0**: keeps `>`, "I did not change [...] `MAX_ORDER`" | **1**: examines `MAX_ORDER`, gets the 4.18 meaning right, still keeps `>` |
| 3. Build and test | 0: malformed hunk, does not apply | 1: well-formed hunk; applies with whitespace fixing and a -3 offset |
| 4. Unsupported or fabricated claims | 0: "applies with `patch -p1`"; "no functional differences are required" | 0: "the CVE fix predates that semantic change"; wrong hunk line filed as "checked" |
| 5. Clarity and reviewability | 1 | 2: separates checked, from knowledge and to be checked by an engineer |
| **Total** | **3/10** | **5/10** |
| Operator in the patch | `>` (wrong) | `>` (wrong) |
| Claims of having built or tested | none | none, and it says so explicitly |

## Did the revised prompt improve the result?

**It improved the analysis and the report, but not the backport. Both
patches are wrong for 4.18.**

What improved, with the evidence:

- **It looked at semantics instead of names (requirement A).** v1-a
  stopped at the name: "The existing 4.18 tree provides the required
  `get_order()` and `MAX_ORDER` interfaces." v2-a examined the meaning and
  got it right for 4.18: "`MAX_ORDER` has the older 'one past the largest
  buddy order' semantics. Thus a requested order equal to `MAX_ORDER` is
  already outside the allocatable range." It also found the redefinition
  series.
- **The report is reviewable (requirement C).** v2-a separates facts that
  were checked, facts from its own knowledge and checks for the engineer.
  Its checklist item "`MAX_ORDER` is still one greater than the maximum
  valid buddy order" (`include/linux/mmzone.h`) would lead a reviewer to
  the bug. v1-a's report said the fix "has been backported" with nothing
  marked as unverified.
- **The patch is closer to applying (requirement B).** The hunk is well
  formed, where v1-a's was malformed, and it applies once whitespace is
  fixed.

What did not improve:

- **The final decision is still wrong, now with a wrong justification.**
  v2-a keeps `>` because "the CVE fix predates that semantic change". The
  redefinition went into v6.4, and its own citation is dated 2023-03-15.
  The fix is from September 2023, for v6.7. So the fix was written for the
  new, inclusive `MAX_ORDER`. The answer also contradicts itself: it says
  an order equal to `MAX_ORDER` is invalid on 4.18, then keeps a check
  that lets that order through.
- **Web search pulled in the wrong authority.** v2-a calls the 4.19 stable
  backport "particularly useful independent confirmation". That backport
  has the same off-by-one. v2-a reached the right semantics and then
  deferred to a precedent that is wrong in the same way.
- **Fabrication moved rather than disappeared.** Neither run claimed to
  have run anything, and v2-a says so explicitly. But v2-a still states
  wrong facts in its body (the timeline, the hunk line), even when the
  report files them under "not checked". Under criterion 4 it scores the
  same 0 as v1-a.

## How much the evidence supports this

- **One run per prompt.** The answers vary between runs, so part of the
  difference may be chance. A second run of each (v1-b, v2-b) would show
  that.
- **Web search could not be turned off.** Both runs used it, so the
  conditions match, but the input was not only the bundle. In v2-a,
  search also changed the outcome (the 4.19 precedent).
- **The model is not identified.** "Account default" may change over time.
  Both runs were 15 minutes apart, on the same account.
- **The scorer knows the reference answer.** I (Claude, who did the Task 3
  backport) scored both runs. The criteria were fixed before the runs,
  and each score quotes the answer, so it can be checked, but the
  judgement on criteria 1 and 5 is still mine.
- **The whitespace problem** (tabs to spaces) is probably from copying out
  of the chat UI. It was not counted against either run.

## Remaining risks and checks that still need an engineer

- **The bound itself.** Neither answer is safe to merge. The fix needs
  `>= MAX_ORDER`, checked against `include/linux/mmzone.h:29`
  (`MAX_ORDER 11`) and `mm/page_alloc.c:4949` (the WARN on
  `order >= MAX_ORDER`). This is what the Task 3 backport uses.
- **Timelines behind a semantic change.** An engineer must check when an
  API changed relative to when the fix was written, from the commits, not
  from the model's account.
- **Stable backports as precedent.** A stable backport to an older series
  is not proof of correctness: the 4.19 one kept the off-by-one.
- **The runtime test.** It needs an AUX-capable PMU (Intel PT). The test VM
  has none, so neither the Task 3 fix nor these answers were exercised at
  run time.

## Same weakness in the Task 3 agent

The Task 3 backports were done with an AI agent (Claude Code). Its mistakes
show the same pattern: a result that looked like validation without proof
that it reached the bug path. The user caught each one from the output:

| Agent mistake | How it was caught |
|---|---|
| hfsc test copied from upstream never sent packets through the qdisc | the same behaviour on both kernels |
| ext4 test counted moved blocks by file content, which move_extent preserves | `blocks moved: 0` on both kernels |
| inode test compared bpftrace `ksym()` names, which don't resolve `r`/`R` data symbols | a false FAIL on the patched kernel |

This is why the rubric weighs "unsupported claims" and "suitability"
separately from "it builds".

## A possible next revision (not run)

v2-a failed on how it weighed evidence, not on finding it. A v3 could:

- ask for the date or version of every semantic change it relies on, next
  to the date of the fix, and the conclusion to follow from that ordering;
- say that stable backports can carry the same bug, so they are not
  evidence that a bound is right;
- ask it to point out contradictions in its own reasoning before giving a
  final answer.

The more effective change is probably in the input rather than the
prompt: adding the target's `MAX_ORDER` definition and the allocator's
`order >= MAX_ORDER` check to the bundle. That was left out here on
purpose, to test whether the model would ask for it. v2-a did ask for it,
in its checklist.
