#!/usr/bin/env bash
# Build a release ISO with the amd64 binary + example configs for one-shot setup.
# Usage: scripts/build-release-iso.sh <version> <dist-dir>
# Expects binaries already built in <dist-dir> as debian-network-tui-<ver>-linux-<arch>.
set -euo pipefail

VERSION="${1:?version required, e.g. v0.3.7}"
DIST="${2:?dist directory required}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

ISO_ROOT="$(mktemp -d)"
trap 'rm -rf "$ISO_ROOT"' EXIT

AMD64="${DIST}/debian-network-tui-${VERSION}-linux-amd64"
if [[ ! -f "$AMD64" ]]; then
  echo "missing amd64 binary: $AMD64" >&2
  exit 1
fi

mkdir -p "${ISO_ROOT}/bin" "${ISO_ROOT}/packages"

# Primary binary at ISO root (copy next to configs for SelfDir()).
install -m 755 "$AMD64" "${ISO_ROOT}/debian-network-tui"

# Extra architectures under bin/
for arch in amd64 arm64 386 arm; do
  src="${DIST}/debian-network-tui-${VERSION}-linux-${arch}"
  if [[ -f "$src" ]]; then
    install -m 755 "$src" "${ISO_ROOT}/bin/debian-network-tui-linux-${arch}"
  fi
done

# Example configs with the names expected beside the binary.
cp "${ROOT}/examples/resolv.conf" "${ISO_ROOT}/resolv.conf"
cp "${ROOT}/examples/sources.list" "${ISO_ROOT}/sources.list"
cp "${ROOT}/examples/ssh-root.conf" "${ISO_ROOT}/ssh-root.conf"
cp "${ROOT}/examples/root.pub" "${ISO_ROOT}/root.pub"

cat > "${ISO_ROOT}/packages/README.txt" <<'EOF'
Place local .deb packages here when preparing a custom ISO or working directory:

  ifenslave_*.deb
  vlan_*.deb
  net-tools_*.deb

  openssh-server_*.deb          (optional; otherwise apt is used)
  openssh-client_*.deb          (required with local openssh-server)
  openssh-sftp-server_*.deb
  runit-helper_*.deb
  libssl3_*.deb
  libwrap0_*.deb

For one-shot / SSH setup, copy these .deb files into the same directory as
debian-network-tui (ISO root), not only into this packages/ folder.
Use matching suite/version for all OpenSSH-related packages.
EOF

cat > "${ISO_ROOT}/README.txt" <<EOF
debian-network-tui ${VERSION} — setup ISO
========================================

Contents
--------
  debian-network-tui   Linux amd64 binary
  bin/                 Other architectures
  resolv.conf          DNS template → /etc/resolv.conf
  sources.list         APT sources template
  ssh-root.conf        Points to root.pub
  root.pub             REPLACE with your real OpenSSH public key
  packages/            Drop ifenslave/vlan/net-tools .deb here (see packages/README.txt)

Quick start (Debian)
--------------------
  # Mount
  mkdir -p /mnt/dntui
  mount -o loop /path/to/debian-network-tui-${VERSION}.iso /mnt/dntui

  # Copy to a writable directory (ISO is read-only)
  mkdir -p /root/dntui-setup
  cp -a /mnt/dntui/. /root/dntui-setup/
  umount /mnt/dntui

  # Edit configs (especially root.pub), add local .deb packages
  cd /root/dntui-setup
  # nano root.pub
  # cp /path/to/ifenslave_*.deb /path/to/vlan_*.deb /path/to/net-tools_*.deb .

  chmod +x debian-network-tui
  sudo ./debian-network-tui
  # Main menu → One-shot setup (DNS, apt, bond/vlan, SSH)

Docs: https://github.com/Songxwn/Debian-network-tui
EOF

ISO_OUT="${DIST}/debian-network-tui-${VERSION}.iso"
if command -v genisoimage >/dev/null 2>&1; then
  genisoimage -V "DNTUI-${VERSION}" -J -r -o "${ISO_OUT}" "${ISO_ROOT}"
elif command -v mkisofs >/dev/null 2>&1; then
  mkisofs -V "DNTUI-${VERSION}" -J -r -o "${ISO_OUT}" "${ISO_ROOT}"
elif command -v xorriso >/dev/null 2>&1; then
  xorriso -as mkisofs -V "DNTUI-${VERSION}" -J -r -o "${ISO_OUT}" "${ISO_ROOT}"
else
  echo "need genisoimage, mkisofs, or xorriso" >&2
  exit 1
fi

(
  cd "${DIST}"
  sha256sum "debian-network-tui-${VERSION}.iso" > "debian-network-tui-${VERSION}.iso.sha256"
)

echo "Wrote ${ISO_OUT}"
ls -lh "${ISO_OUT}" "${ISO_OUT}.sha256"
