# Build environment for the CentOS Stream 8 kernel SRPM.
#
# The image is "warm": build dependencies of the reference SRPM are installed
# at image build time, so repeated builds (original, patched, iterations) do
# not hit the slow vault mirrors. build.sh re-runs builddep at runtime against
# the SRPM actually being built, which is a no-op unless BuildRequires changed.

# Pinned by digest for reproducibility (quay.io/centos/centos:stream8).
ARG BASE_IMAGE=quay.io/centos/centos@sha256:20da069d4f8126c4517ee563e6e723d4cbe79ff62f6c4597f753478af91a09a3
FROM ${BASE_IMAGE}

# Reference SRPM used to pre-install build dependencies.
ARG SRPM_URL=https://vault.centos.org/8-stream/BaseOS/Source/SPackages/kernel-4.18.0-448.el8.src.rpm
ARG SRPM_SHA256=0e0533ea6d03a7a35f5066e4b2bde617670acb0bcc2d92fe7fd6374109152a87

# Stream 8 is EOL: replace the stock mirrorlist-based repos with vault ones.
RUN rm -f /etc/yum.repos.d/*.repo
COPY vault.repo /etc/yum.repos.d/vault.repo

RUN dnf -y install dnf-plugins-core rpm-build rpmdevtools \
 && dnf clean all

RUN curl -fsSL --retry 5 -o /tmp/ref.src.rpm "${SRPM_URL}" \
 && echo "${SRPM_SHA256}  /tmp/ref.src.rpm" | sha256sum -c - \
 && dnf -y builddep /tmp/ref.src.rpm \
 && rm -f /tmp/ref.src.rpm \
 && dnf clean all

# Unprivileged build user; builddep at runtime still runs as root.
RUN useradd -m -u 1000 builder

COPY build.sh /usr/local/bin/build.sh
RUN chmod 0755 /usr/local/bin/build.sh

VOLUME ["/input", "/output"]
ENTRYPOINT ["/usr/local/bin/build.sh"]
