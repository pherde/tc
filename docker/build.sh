#!/bin/bash
# Container entrypoint: build binary RPMs from a kernel SRPM.
#
# Contract:
#   input : exactly one *.src.rpm in /input (or its path as $1)
#   output: /output/RPMS/<arch>/*.rpm, build.log, manifest.txt,
#           toolchain.txt, SHA256SUMS
#
# Environment:
#   JOBS           parallel make jobs            (default: nproc)
#   RPMBUILD_OPTS  extra rpmbuild options        (default: lean build)
#   HOST_UID/GID   owner of exported files       (default: builder)
#   IMAGE_REF      image reference for manifest  (set by the caller)
#
# Exit codes:
#   2 invalid input   3 build dependencies   4 rpmbuild failed   5 export failed

set -euo pipefail

readonly OUT=/output
readonly TOPDIR=/home/builder/rpmbuild
readonly DEFAULT_OPTS="--with baseonly --without debuginfo --without kabidwchk"

JOBS=${JOBS:-$(nproc)}
RPMBUILD_OPTS=${RPMBUILD_OPTS:-$DEFAULT_OPTS}
HOST_UID=${HOST_UID:-$(id -u builder)}
HOST_GID=${HOST_GID:-$(id -g builder)}

log() { echo "==> $*"; }
die() { local code=$1; shift; echo "ERROR: $*" >&2; exit "$code"; }

fix_owner() { chown -R "$HOST_UID:$HOST_GID" "$OUT" 2>/dev/null || true; }
trap fix_owner EXIT

# ---- input -----------------------------------------------------------------
if [[ $# -ge 1 ]]; then
    SRPM=$1
else
    shopt -s nullglob
    srpms=(/input/*.src.rpm)
    shopt -u nullglob
    [[ ${#srpms[@]} -eq 1 ]] || die 2 "expected exactly one *.src.rpm in /input, found ${#srpms[@]}"
    SRPM=${srpms[0]}
fi
[[ -f $SRPM ]] || die 2 "SRPM not found: $SRPM"
[[ -d $OUT && -w $OUT ]] || die 5 "$OUT is not a writable directory (missing volume or SELinux label?)"

exec > >(tee "$OUT/build.log") 2>&1

log "SRPM: $SRPM"
rpm --import /etc/pki/rpm-gpg/RPM-GPG-KEY-centosofficial
rpm -K "$SRPM" || die 2 "SRPM integrity check failed: $SRPM"
# %{ARCH} of an SRPM is the arch it was built on; SOURCEPACKAGE is the flag.
rpm -qp --qf '%{SOURCEPACKAGE}\n' "$SRPM" 2>/dev/null | grep -qx 1 \
    || die 2 "not a source RPM: $SRPM"
NVR=$(rpm -qp --qf '%{NAME}-%{VERSION}-%{RELEASE}' "$SRPM")
log "NVR: $NVR"

# ---- dependencies ----------------------------------------------------------
# No-op on a warm image unless the SRPM changed its BuildRequires.
log "installing build dependencies"
dnf -y builddep "$SRPM" || die 3 "dnf builddep failed (vault reachable?)"

# ---- build -----------------------------------------------------------------
rm -rf "$TOPDIR"
install -d -o builder -g builder "$TOPDIR"
cp "$SRPM" /tmp/input.src.rpm
chmod 0644 /tmp/input.src.rpm

log "rpmbuild options: $RPMBUILD_OPTS (jobs: $JOBS)"
start=$(date +%s)
# shellcheck disable=SC2086  # RPMBUILD_OPTS is intentionally word-split
runuser -u builder -- bash -c "
    set -euo pipefail
    rpm -i --define '_topdir $TOPDIR' /tmp/input.src.rpm
    rpmbuild -bb --target x86_64 \
        --define '_topdir $TOPDIR' \
        --define '_smp_mflags -j$JOBS' \
        $RPMBUILD_OPTS \
        $TOPDIR/SPECS/kernel.spec
" || die 4 "rpmbuild failed, see $OUT/build.log"
elapsed=$(( $(date +%s) - start ))

# ---- export ----------------------------------------------------------------
log "exporting RPMs"
rm -rf "$OUT/RPMS"
cp -r "$TOPDIR/RPMS" "$OUT/RPMS" || die 5 "copying RPMs to $OUT failed"
count=$(find "$OUT/RPMS" -name '*.rpm' | wc -l)
[[ $count -gt 0 ]] || die 5 "rpmbuild produced no RPMs"

(cd "$OUT" && find RPMS -name '*.rpm' | sort | xargs sha256sum > SHA256SUMS)
rpm -qa | sort > "$OUT/toolchain.txt"

cat > "$OUT/manifest.txt" <<EOF
nvr:            $NVR
srpm:           $(basename "$SRPM")
srpm_sha256:    $(sha256sum "$SRPM" | cut -d' ' -f1)
image:          ${IMAGE_REF:-unknown}
rpmbuild_opts:  $RPMBUILD_OPTS
jobs:           $JOBS
gcc:            $(rpm -q gcc)
binutils:       $(rpm -q binutils)
build_seconds:  $elapsed
finished_utc:   $(date -u +%Y-%m-%dT%H:%M:%SZ)
rpm_count:      $count
EOF

log "done: $count RPMs in $OUT/RPMS (${elapsed}s)"
