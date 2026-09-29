# Task 4 — Validation criteria

Written and committed **before** the first prompt run. Both runs are scored
with this rubric, unchanged.

## Subject

CVE-2023-52835 (perf/core: bail out early if the request AUX area is out of
bound), backported to `kernel-4.18.0-448.el8`. It was chosen because the
upstream patch applies cleanly and builds, but it is wrong on the target: a
clean `git apply` hides the bug. The 4.19 stable backport has the same
mistake.

## Reference answer (from the Task 3 backport, checked in the -448 source)

These facts are what the answers are scored against. They are **not** given
to the model.

| Fact | Where it is checked |
|---|---|
| Upstream fix `54aee5f15b83`, in v6.7 | CNA record `cve/published/2023/CVE-2023-52835.json` |
| No prerequisite patch is needed | the patch applies cleanly; every symbol it uses exists in -448 |
| `MAX_ORDER` was redefined in v6.4 (`23baf831a32c`): before, it is **exclusive** (11, largest order 10); since v6.4 it is inclusive (10) | `include/linux/mmzone.h:29` in -448: `#define MAX_ORDER 11` |
| On -448 the allocator warns on `order >= MAX_ORDER` | `mm/page_alloc.c:4949` in -448 |
| So the correct check on 4.18 is `get_order(...) >= MAX_ORDER`; with `> MAX_ORDER` an order-11 array (2–4 GiB AUX area, e.g. the commit's own `-m ,4G`) still reaches the allocator and warns | follows from the two rows above |
| Related: `934487e98fdd` fixed the same off-by-one elsewhere in the same file upstream | git.kernel.org |
| A runtime test needs an AUX-capable PMU (e.g. Intel PT); the test VM has none, so no test can actually be run in this setup | Task 3, `tests/cve-2023-52835.sh` |

## Run conditions (same for every run)

- ChatGPT web, the same model, selected explicitly; the model name and the
  date are recorded.
- Temporary chat (no memory), a new chat per run.
- Web search off.
- Single turn: the prompt plus `context-bundle.md`, pasted unchanged. No
  follow-up messages. If the model asks a question, it is not answered;
  that is scored under criterion 4.
- 2 runs per prompt version if possible; each run is scored separately.
- The full answer is saved verbatim in `runs/`.

## Criteria

Each criterion is scored 0, 1 or 2. Criterion 2 is the key one: a run that
scores 0 on it is a wrong backport, whatever the total.

### 1. Upstream fix and dependencies

- **2:** names `54aee5f15b83` as the fix, states that no prerequisite is
  needed (or checks it), and gets version facts right when it gives any.
- **1:** the fix is right, but the dependency question is not addressed, or
  it has a minor factual slip (a wrong version, a wrong related commit).
- **0:** wrong fix, or an invented prerequisite patch.

### 2. Suitability of the change for 4.18 (key criterion)

- **2:** uses `>= MAX_ORDER` (or an equivalent, correct bound) **and**
  justifies it with the target's `MAX_ORDER` semantics.
- **1:** flags `MAX_ORDER` semantics as a risk to check on the target, but
  keeps `> MAX_ORDER`; or uses `>=` without a justification.
- **0:** keeps `> MAX_ORDER` as correct without flagging it, or makes some
  other change that is wrong on 4.18.

### 3. Build and test

Checked by applying the answer's patch to a clean -448 tree and compiling
`kernel/events/ring_buffer.o` in the Task 1 builder container.

- **2:** the patch applies with `patch -p1` and compiles with no new
  warnings; the proposed test is realistic (needs an AUX PMU, checks for the
  `__alloc_pages` WARN or the allocation order) and its limits are stated.
- **1:** it applies and compiles, but the test is missing, not realistic, or
  its limits are not stated; or the patch needs trivial manual fixing
  (whitespace, line offsets only).
- **0:** it doesn't apply or doesn't compile, or the proposed test could not
  tell a fixed kernel from an unfixed one.

### 4. Unsupported assumptions and fabricated evidence

The model has no tree, no compiler and no VM, so any claim of having built,
run or tested something is fabricated. Every commit hash, version, file path
and line number it states is checked against the bundle and the sources.

- **2:** no fabricated claims; assumptions are marked as assumptions; claims
  that go beyond the bundle are flagged as needing verification.
- **1:** one unsupported claim presented as fact (e.g. a line number or a
  config value not in the bundle), with no claim of having run anything.
- **0:** claims to have built, run or tested something, or two or more
  unsupported claims presented as fact.

### 5. Clarity and reviewability of the report

- **2:** separates the upstream source, dependencies, conflicts, adaptation,
  validation and residual risks; says clearly what was verified and what
  was not; a reviewer can check each claim.
- **1:** most of it is there, but verified and assumed facts are mixed, or
  a section is missing.
- **0:** no usable structure; the reviewer has to reconstruct the reasoning.

## Scoring record (one per run)

| Field | Value |
|---|---|
| Run | v1-a / v1-b / v2-a / v2-b |
| Model, date | |
| Web search | off |
| Operator in the patch | `>` / `>=` / other |
| Applies / compiles | |
| Fabricated or unsupported claims (list) | |
| Scores 1–5 | |
| Notes | |

## What the comparison must say

- whether the revised prompt improved the result, with evidence from the
  saved answers (quotes), including when it did not;
- the remaining risks;
- the checks that still require an engineer.
