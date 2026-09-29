#!/bin/bash
# Generate the patched SRPM kernel-4.18.0-448.el8.cvefix8.src.rpm from the
# original kernel-4.18.0-448.el8.src.rpm, the 9 patches in patches/ and
# patches/kernel.spec.diff.
#
# It runs in the Task 1 builder container (same rpm and toolchain as the
# kernel build): unpacks the original SRPM, adds the patches, applies the
# spec diff, checks %prep with rpmbuild -bp, and writes the SRPM with
# rpmbuild -bs.
#
# Usage: scripts/make-patched-srpm.sh <original.src.rpm> <outdir>
#   IMAGE=<image>  builder image (default stream8-kernel-builder:latest)
#   SKIP_PREP=1    skip the rpmbuild -bp check (~30 s)
#
# Exit codes: 1 usage, 2 bad input, 3 spec diff, 4 %prep, 5 rpmbuild -bs.

set -euo pipefail

ORIG_SHA256=0e0533ea6d03a7a35f5066e4b2bde617670acb0bcc2d92fe7fd6374109152a87
IMAGE=${IMAGE:-stream8-kernel-builder:latest}
REPO=$(cd "$(dirname "$0")/.." && pwd)

[ $# -eq 2 ] || { echo "usage: $0 <original.src.rpm> <outdir>" >&2; exit 1; }
SRPM=$(realpath "$1")
OUT=$2
[ -f "$SRPM" ] || { echo "not found: $SRPM" >&2; exit 2; }
echo "$ORIG_SHA256  $SRPM" | sha256sum -c --quiet - \
	|| { echo "$SRPM is not the original kernel-4.18.0-448.el8.src.rpm (SHA-256 mismatch)" >&2; exit 2; }
mkdir -p "$OUT"
OUT=$(realpath "$OUT")

docker run --rm --user builder --entrypoint bash \
	-e SKIP_PREP="${SKIP_PREP:-0}" \
	-v "$SRPM:/in/orig.src.rpm:ro,Z" \
	-v "$REPO/patches:/in/patches:ro,Z" \
	-v "$OUT:/out:Z" \
	"$IMAGE" -c '
set -euo pipefail
TOP=/tmp/rpmbuild
rpm -i --define "_topdir $TOP" /in/orig.src.rpm
cp /in/patches/100[0-9]-*.patch $TOP/SOURCES/
cd $TOP
patch -p1 --quiet < /in/patches/kernel.spec.diff || exit 3
if [ "$SKIP_PREP" != 1 ]; then
	rpmbuild --define "_topdir $TOP" -bp --target x86_64 SPECS/kernel.spec > /tmp/prep.log 2>&1 \
		|| { tail -20 /tmp/prep.log; exit 4; }
	echo "%prep: $(grep -c "^+ ApplyPatch 100" /tmp/prep.log) patches applied"
fi
rpmbuild --define "_topdir $TOP" -bs SPECS/kernel.spec | tail -1 || exit 5
cp SRPMS/*.src.rpm /out/
'

cd "$OUT"
for f in kernel-4.18.0-448.el8.cvefix8.src.rpm; do
	[ -f "$f" ] || { echo "missing $f" >&2; exit 5; }
	echo "--- $f"
	sha256sum "$f"
	rpm -qlp "$f" | grep '^100'
done
