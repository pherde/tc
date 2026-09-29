# Prompt v2 (revised)

Paste the text between the lines, then paste the whole of
`context-bundle.md` right after it, in the same message. The bundle is the
same as in v1, unchanged.

The first paragraph and the four numbered requests are the v1 prompt,
unchanged. The "Requirements" section is new. Each requirement targets a
weakness observed in run v1-a (see `runs/v1-a-score.md`):

| Requirement | v1-a weakness | Criterion |
|---|---|---|
| A. Semantics, not names | `MAX_ORDER` existed in the target, so v1-a assumed it meant the same | 2 |
| B. Patch built from the numbered target code | malformed hunk header; "applies with `patch -p1`" stated without basis | 3, 4 |
| C. Verified / assumed / to check | assumptions presented as facts; report said "has been backported" | 4, 5 |

---

I need to backport the fix for CVE-2023-52835 to a RHEL 8 based kernel,
`kernel-4.18.0-448.el8` (CentOS Stream 8, x86_64). The upstream fix and the
current code of the affected function in my tree are below.

Please:

1. Give me the backported patch for my tree, in unified diff format that
   applies with `patch -p1`.
2. Explain what you changed compared to the upstream patch, if anything,
   and why.
3. Tell me how to build and test it.
4. Write a short backport report I can attach to the change.

Requirements:

A. The upstream fix was written for a much newer kernel than 4.18. For
   every macro, constant, function and type that the fix uses or relies on,
   do not stop at checking that the name exists in 4.18: check whether its
   definition or meaning changed between 4.18 and the kernel version where
   the fix landed. If a changed meaning affects the fix, adapt the fix and
   explain why. If you can't tell from the code I gave you, say so, and give
   me the exact file and check to run in my tree.

B. Build the diff from the numbered target code above: the context lines,
   the line numbers in the hunk header and the tab indentation must match
   my file. If you can't be sure that it applies, say so.

C. You have no access to my tree, a compiler or a test machine. Don't
   write that anything was built, applied, run or tested. In the report,
   split the facts into three groups: checked in the code I gave you; from
   your own knowledge, not checked here; and to be checked by an engineer
   before merging.

---
