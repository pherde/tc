# CentOS Stream 8 Kernel Build, Patch Backporting, and Prompt Validation

Solution to the Kernel Developer test assignment. The target is the CentOS
Stream 8 kernel SRPM `kernel-4.18.0-448.el8.src.rpm`.

| Task | Status | Section |
|---|---|---|
| 1. Build the kernel SRPM in Docker | ✅ Done | [Task 1](#task-1--build-the-kernel-srpm-in-docker) |
| 2. Go build tool | ⏳ Pending | [Task 2](#task-2--go-build-tool) |
| 3. Backport security fixes and rebuild | ⏳ Pending | [Task 3](#task-3--backport-security-fixes-and-rebuild) |
| 4. Improve and validate a backporting prompt | ⏳ Pending | [Task 4](#task-4--improve-and-validate-a-backporting-prompt) |

## Repository layout

```
docker/
  Dockerfile   CentOS Stream 8 build image (pinned by digest), build deps pre-installed
  vault.repo   BaseOS / AppStream / PowerTools repos on vault.centos.org
  build.sh     container entrypoint: validate SRPM, builddep, rpmbuild, export
README.md
```

---

## Task 1 — Build the kernel SRPM in Docker

### Design

- **Base image:** `quay.io/centos/centos:stream8`, pinned by digest. This
  gives the same distribution the SRPM targets, with the EL8 GCC 8 toolchain.
- **Repositories:** Stream 8 is EOL and `mirrorlist.centos.org` is gone, so
  the stock repo files are replaced by `vault.repo`, which points to
  `vault.centos.org`.
- **"Warm" image:** the image build downloads the reference SRPM, checks its
  SHA-256 and runs `dnf builddep` on it, so repeated builds don't hit the slow
  vault. `build.sh` runs `builddep` again at runtime against the SRPM being
  built. This is a no-op unless its `BuildRequires` changed, for example in a
  patched SRPM.
- **Unprivileged build:** `rpmbuild` runs as user `builder`. Only `builddep`
  runs as root.
- **Lean build:** only the base kernel is built. See
  [Build options](#build-options).

### Prerequisites

- An x86_64 Linux host with Docker (tested with Docker Engine 29.1 on Fedora 43).
- About 30 GB of free disk for the image and the build tree. RAM ≥ 8 GB.
- Network access to `quay.io` and `vault.centos.org`.

### Step 1 — Clone the repository

```sh
git clone <repo-url> stream8-kernel
cd stream8-kernel
```

### Step 2 — Build the builder image

```sh
docker build -t stream8-kernel-builder:latest docker/
```

This takes a few minutes, mostly for `dnf builddep` from the vault. To build
the image against a different reference SRPM, use
`--build-arg SRPM_URL=... --build-arg SRPM_SHA256=...`.

### Step 3 — Download the SRPM

```sh
mkdir -p in out/original
curl -fL -o in/kernel-4.18.0-448.el8.src.rpm \
  https://vault.centos.org/8-stream/BaseOS/Source/SPackages/kernel-4.18.0-448.el8.src.rpm
echo "0e0533ea6d03a7a35f5066e4b2bde617670acb0bcc2d92fe7fd6374109152a87  in/kernel-4.18.0-448.el8.src.rpm" \
  | sha256sum -c -
```

`in/` must contain exactly one `*.src.rpm`.

### Step 4 — Build the kernel

```sh
docker run --rm \
  -v "$PWD/in:/input:ro,Z" \
  -v "$PWD/out/original:/output:Z" \
  -e HOST_UID="$(id -u)" -e HOST_GID="$(id -g)" \
  -e IMAGE_REF="stream8-kernel-builder:latest@$(docker image inspect -f '{{.Id}}' stream8-kernel-builder:latest)" \
  stream8-kernel-builder:latest
```

- `:Z` relabels the volumes for SELinux (Fedora/RHEL hosts). Without it the
  container can't write to `/output`.
- `HOST_UID`/`HOST_GID` make the exported files owned by your user.
- `IMAGE_REF` is recorded in `manifest.txt` for traceability.

Reference run: **~23 min** on a 12-thread / 15 GB RAM host.

### Step 5 — Check the artifacts

```
out/original/
  RPMS/x86_64/
    kernel-4.18.0-448.el8.x86_64.rpm
    kernel-core-4.18.0-448.el8.x86_64.rpm
    kernel-modules-4.18.0-448.el8.x86_64.rpm
    kernel-modules-extra-4.18.0-448.el8.x86_64.rpm
    kernel-modules-internal-4.18.0-448.el8.x86_64.rpm
    kernel-devel-4.18.0-448.el8.x86_64.rpm
    kernel-headers-4.18.0-448.el8.x86_64.rpm
  build.log       full rpmbuild log
  manifest.txt    NVR, SRPM SHA-256, image, rpmbuild options, gcc/binutils, duration
  toolchain.txt   rpm -qa of the build container
  SHA256SUMS      checksums of the exported RPMs
```

```sh
cd out/original
sha256sum -c SHA256SUMS
cat manifest.txt
grep -n check-kabi build.log     # kABI symbol check ran
cd ../..
```

### Step 6 — Test the kernel in an AlmaLinux 8.10 VM

A RHEL8-compatible userspace is required. An EL9/EL10 guest (systemd,
dracut, XFS features, RPM dependencies) is not a meaningful target for a
4.18 EL8 kernel. The steps below use an **AlmaLinux 8.10** guest under
QEMU/KVM.

**6.1 — Disable Secure Boot in the VM (UEFI guests only).** The kernel is
signed with pesign's local "Red Hat Test Certificate" (see
[Notes](#notes-and-known-limitations)), which shim doesn't trust. Check with:

```sh
mokutil --sb-state
```

**6.2 — Copy the packages to the VM.** Only four of the seven RPMs are
needed:

```sh
scp out/original/RPMS/x86_64/{kernel,kernel-core,kernel-modules,kernel-modules-extra}-4.18.0-448.el8.x86_64.rpm \
    <user>@<vm-address>:~/
```

| RPM | Install? | Why |
|---|---|---|
| `kernel`, `kernel-core`, `kernel-modules` | yes | kernel image, initramfs, modules |
| `kernel-modules-extra` | yes | less common modules; several CVE subsystems live here |
| `kernel-devel` | optional | only for building external modules |
| `kernel-modules-internal` | no | in-tree test modules (kunit, livepatch) |
| `kernel-headers` | **no** | not install-only: it would *downgrade* the guest's headers package |

**6.3 — Install (in the VM).**

```sh
sudo dnf install ./kernel-*4.18.0-448.el8.x86_64.rpm
# if dnf refuses because 4.18.0-553 (newer) is installed:
sudo rpm -ivh ./kernel-*4.18.0-448.el8.x86_64.rpm
```

The kernel is install-only, so -448 is installed **alongside** the stock
AlmaLinux kernel. The package scriptlets generate the initramfs and the GRUB
entry.

**6.4 — Boot it.** Reboot and choose `4.18.0-448.el8.x86_64` in the GRUB
menu. To make it the default instead:

```sh
sudo grubby --set-default /boot/vmlinuz-4.18.0-448.el8.x86_64
```

**6.5 — Verify (in the VM).** Run as root. The output is also saved to
`/root/k448-check.txt`:

```sh
sudo bash -c '
{
echo "### uname -r";            uname -r
echo; echo "### rpm -qi kernel-core"
rpm -qi kernel-core-4.18.0-448.el8 | grep -E "^(Name|Version|Release|Build Host|Build Date|Signature)"
echo; echo "### kernel cmdline";  cat /proc/cmdline
echo; echo "### systemctl --failed"
systemctl --failed --no-pager
echo; echo "### journalctl -b -p err"
journalctl -b -p err --no-pager
echo; echo "### dmesg: oops / bug / call trace / warnings"
dmesg | grep -iE "oops|bug:|call trace|warning:|taint" || echo "(nothing found)"
echo; echo "### /proc/sys/kernel/tainted"
t=$(cat /proc/sys/kernel/tainted); echo "value: $t"
for i in $(seq 0 31); do [ $(( (t >> i) & 1 )) -eq 1 ] && echo "  bit $i set"; done
echo; echo "### modprobe (subsystems touched by the Task 3 CVEs)"
for m in udf sch_hfsc rtlwifi; do
  modprobe -v $m && echo "$m: OK" || echo "$m: FAILED"
done
lsmod | grep -E "^(udf|sch_hfsc|rtlwifi)"
echo; echo "### modinfo (module signature)"
modinfo udf | grep -E "^(filename|signer|sig_key|vermagic)"
} 2>&1 | tee /root/k448-check.txt
'
```

| Check | What it shows |
|---|---|
| `rpm -qi` | `Build Host` = build container ID and `Signature` = `(none)`: our build, not an official package with the same NVR |
| `/proc/cmdline` | booted from `vmlinuz-4.18.0-448.el8.x86_64` |
| `systemctl --failed --no-pager` | units that failed to start |
| `journalctl -b -p err --no-pager` | messages of priority `err` or worse in the current boot (`-b`) |
| `dmesg \| grep` | kernel oops, `BUG:`, call traces, warnings |
| `/proc/sys/kernel/tainted` | taint flags, decoded bit by bit (`0` = clean) |
| `modprobe -v` | modules of the CVE subsystems load, with their dependencies |
| `modinfo` | modules are signed with the build's module signing key |

**Rollback:** pick the `4.18.0-553` entry in GRUB, or
`sudo grubby --set-default /boot/vmlinuz-4.18.0-553*`.

### Result — original SRPM (AlmaLinux 8.10 guest, QEMU)

| Check | Result |
|---|---|
| `uname -r` | `4.18.0-448.el8.x86_64` |
| `rpm -qi kernel-core` | `Build Host: a7c60a6ff665` (build container), `Signature: (none)` |
| `/proc/cmdline` | `BOOT_IMAGE=(hd0,msdos1)/vmlinuz-4.18.0-448.el8.x86_64 ...` |
| `systemctl --failed` | `0 loaded units listed` |
| `journalctl -b -p err` | `-- No entries --` |
| `dmesg` oops / BUG / call trace / warnings | none |
| `tainted` | `0` |
| `modprobe udf`, `sch_hfsc`, `rtlwifi` | all OK (dependencies loaded too: `crc-itu-t`, `cfg80211`, `mac80211`, ...) |
| `modinfo udf` | `vermagic: 4.18.0-448.el8.x86_64 SMP mod_unload modversions`, `signer: CentOS kernel signing key` |

About the module signer: `CentOS kernel signing key` is only the subject
name. It comes from the SRPM's `x509.genkey` template
(`CN = CentOS kernel signing key`). The key pair is **generated during the
build** (`certs/signing_key.pem`) and exists only inside the build
container. It is not the official CentOS key. The kernel trusts it because
its certificate is built into the same kernel image, which is why the
modules load without tainting the kernel.

### Build options

Default `RPMBUILD_OPTS` (flags checked against the -448 `kernel.spec`):

| Option | Effect |
|---|---|
| `--with baseonly` | base kernel only: no debug kernel, perf, tools, bpftool, selftests, ipaclones |
| `--without debuginfo` | no `-debuginfo` packages |
| `--without kabidwchk` | skip the DWARF-based kABI check |

The symbol-CRC kABI check (`kabichk`) stays **on** on purpose: it flags
backports that change the exported kernel ABI.

| Variable | Default | Meaning |
|---|---|---|
| `RPMBUILD_OPTS` | lean build above | extra `rpmbuild` options |
| `JOBS` | `nproc` | make parallelism |
| `HOST_UID` / `HOST_GID` | `builder` (1000) | owner of exported files |
| `IMAGE_REF` | `unknown` | image reference written to `manifest.txt` |

### Exit codes (`build.sh`)

| Code | Meaning |
|---|---|
| 0 | success |
| 2 | invalid input: no or multiple SRPMs, bad digest/signature, not a source RPM |
| 3 | `dnf builddep` failed (vault unreachable?) |
| 4 | `rpmbuild` failed (see `build.log`) |
| 5 | export failed: output not writable, no RPMs produced |

### Notes and known limitations

- **Toolchain fidelity:** the vault holds the *final* Stream 8 package set
  (2024), not the January 2023 set -448 was originally built with. The exact
  toolchain is recorded in `manifest.txt` / `toolchain.txt` (gcc 8.5.0-22,
  binutils 2.30-123).
- **Secure Boot:** the spec's `%pesign` falls back to the local "Red Hat Test
  Certificate" because the CentOS signing keys aren't available.
- `-fdump-ipa-clones` is still passed with `--with baseonly`, because the spec
  sets `kpatch_kcflags` before the `baseonly` block clears `with_ipaclones`.
  This is harmless.
- Benign warnings from the upstream spec: `user/group mockbuild does not
  exist`, `File listed twice` (kunit/livepatch test modules).
- `kabi-dw` is installed in the image, so `kabidwchk` can in principle be
  re-enabled with `-e RPMBUILD_OPTS="--with baseonly --without debuginfo"`
  (not tested yet).

---

## Task 2 — Go build tool

_Pending._

## Task 3 — Backport security fixes and rebuild

_Pending._

## Task 4 — Improve and validate a backporting prompt

_Pending._
