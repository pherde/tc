# Prompt v1 (initial)

Paste the text between the lines, then paste the whole of
`context-bundle.md` right after it, in the same message.

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

---
