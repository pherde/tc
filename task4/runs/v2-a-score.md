# Score — run v2-a

Scored with `criteria.md` (commit `3bfc733`, made before both runs). Prompt
v2 was committed (`082fe0e`) before this run.

| Field | Value |
|---|---|
| Run | v2-a |
| Model, date | ChatGPT web, account default (same account as v1-a), 2026-09-29 12:43 |
| Web search | used, as in v1-a (lkml, lore, openwall and NVD citations) |
| Operator in the patch | **`>`** (the upstream check, unchanged) |
| Applies / compiles | as delivered, **`Hunk #1 FAILED at 699`**; with `-l` (ignore whitespace) it applies, `Hunk #1 succeeded at 696 (offset -3 lines)`. The hunk is well formed. The change is the same as v1-a's, which compiles with no warnings |
| Scores | 1: **1** · 2: **1** · 3: **1** · 4: **0** · 5: **2** — total 5/10 |

## Checks

**Patch applicability**, against the unpatched `kernel/events/ring_buffer.c`
of -448 (baseline commit `cbcacafea`):

```
$ patch -p1 --dry-run < v2a.patch
Hunk #1 FAILED at 699.
$ patch -p1 -l --dry-run < v2a.patch
Hunk #1 succeeded at 696 (offset -3 lines).
```

Tabs came out as spaces again, as in v1-a. That may come from copying out
of the chat UI, so it is not counted against the model. The hunk now
carries 6 old lines, matching its header; v1-a's did not. The header line
is wrong, though: the context starts at line 696, and the answer put 699.

**Build.** The inserted lines are the same as v1-a's, and that change
compiles with no warnings in the builder container (see
`v1-a-score.md`). It was not rebuilt.

## Criterion by criterion

**1. Upstream fix and dependencies — 1.** The fix is right, and so is the
statement that the 4.19 stable backport kept the same six lines. But the
version is wrong:

> The upstream fix landed in the 6.6-era kernel

The CNA record gives v6.7. The 6.6 link it cites is an AUTOSEL stable
backport, not the mainline commit.

**2. Suitability for 4.18 — 1.** A clear change from v1-a: it examines
`MAX_ORDER` semantics and states the 4.18 meaning correctly.

> in the 4.18/6.6-era kernels, `MAX_ORDER` has the older "one past the
> largest buddy order" semantics. Thus a requested order equal to
> `MAX_ORDER` is already outside the allocatable range.

It also cites the right commit (the "Redefine MAX_ORDER sanely" series),
and it puts "`MAX_ORDER` is still one greater than the maximum valid buddy
order" in the engineer's checklist. But it keeps `> MAX_ORDER`, based on a
wrong timeline:

> The CVE fix predates that semantic change and therefore its
> `> MAX_ORDER` expression is appropriate for the semantics of the target
> generation.

The redefinition went into v6.4. Its own citation [3] is dated
2023-03-15, and the fix was written in September 2023 for v6.7. So the
fix was written for the *new* (inclusive) `MAX_ORDER`. The answer also
contradicts itself: it says an order equal to `MAX_ORDER` is outside the
allocatable range on 4.18, and then keeps a check (`>`) that lets exactly
that order through. It also takes the 4.19 stable backport as
"particularly useful independent confirmation", but that backport has the
same off-by-one. The rubric level is 1: it flags `MAX_ORDER` semantics as
a risk to check, but keeps `>`.

**3. Build and test — 1.** It applies only with whitespace fixing and a -3
line offset; that is the rubric's "trivial manual fixing". The test plan is
sound but generic: it says the ARM SPE reproducer doesn't fit x86_64 and
states the expected behaviour (`-ENOMEM`, no allocator warning). It does
not name Intel PT, which v1-a did. It states its limits ("I have not
built, applied, or tested this patch").

**4. Unsupported or fabricated claims — 0.** No claim of having built, run
or tested anything, which is an improvement over v1-a. But it presents at
least two claims as fact that are wrong:
- "The CVE fix predates that semantic change": false (above). The
  report's "from knowledge, not checked" group does list it, but the body
  states it as fact and bases the decision on it;
- in the "checked in the code supplied" group: "The hunk is adjusted from
  upstream's `@@ -700` to your target's `@@ -699`, because your
  `kcalloc_node()` statement is at line 699". The hunk starts at the first
  context line (696), not at the call, so the header is off by 3.

It also says "later stable kernels added separate validation for
excessively large `nr_pages`" without a source. That was not verified here.

**5. Clarity — 2.** It has the three groups the prompt asked for (checked
in the given code; from knowledge; to be checked by an engineer), a
per-identifier semantics table, and a checklist with file paths. A reviewer
can check every claim. Following the checklist (item 2: "`MAX_ORDER` is
still one greater than the maximum valid buddy order", in
`include/linux/mmzone.h`) would lead the engineer to the bug. The
"Assessment" still concludes that no adaptation is needed.

## What changed from v1-a

- Requirement A worked partly. The model looked at `MAX_ORDER`
  semantics, got the 4.18 meaning right and found the redefinition
  commit. It then made a timeline error and trusted the 4.19 stable
  backport over its own reasoning, so the patch is still wrong.
- Requirement B worked partly. The hunk is now well formed, but the header
  line is still wrong, and tabs still come out as spaces (maybe the UI).
- Requirement C worked for the "no fabricated runs" part, and the
  report's structure is much better. Wrong facts are still asserted in the
  body, though, even when the report files them as "not checked".
