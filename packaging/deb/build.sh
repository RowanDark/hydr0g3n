#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <path-to-binary> <version> [architecture]" >&2
  exit 1
fi

BINARY_PATH=$1
VERSION=$2
ARCH=${3:-amd64}
PKG_NAME=hydr0g3n
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
WORK_DIR=$(mktemp -d)
PACKAGE_ROOT="${WORK_DIR}/${PKG_NAME}_${VERSION}_${ARCH}"

mkdir -p "${PACKAGE_ROOT}/DEBIAN"
mkdir -p "${PACKAGE_ROOT}/usr/local/bin"
mkdir -p "${PACKAGE_ROOT}/usr/share/doc/${PKG_NAME}"

sed -e "s/@VERSION@/${VERSION}/" \
    -e "s/@ARCH@/${ARCH}/" \
    "${SCRIPT_DIR}/DEBIAN/control" > "${PACKAGE_ROOT}/DEBIAN/control"

install -m 0755 "${BINARY_PATH}" "${PACKAGE_ROOT}/usr/local/bin/hydro"
install -m 0644 "${SCRIPT_DIR}/../../LICENSE" "${PACKAGE_ROOT}/usr/share/doc/${PKG_NAME}/copyright"

fakeroot dpkg-deb --build "${PACKAGE_ROOT}" >/dev/null

OUTPUT_PATH="${PACKAGE_ROOT}.deb"
DEST_DIR="${SCRIPT_DIR}"

mv "${OUTPUT_PATH}" "${DEST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb"

echo "Created ${DEST_DIR}/${PKG_NAME}_${VERSION}_${ARCH}.deb"
