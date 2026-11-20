#!/bin/sh
# Generates the Windows resources (icon, manifest and version information)
# that the Go linker adds when building rho-gui for Windows. Run from
# anywhere with the version as the argument, e.g. "sh syso.sh 1.2.3".
set -eu
version=${1:-0.0.0}
here=$(cd "$(dirname "$0")" && pwd)
info=$(mktemp)
trap 'rm -f "$info"' EXIT
cat > "$info" <<JSON
{
  "fixed": {"file_version": "$version", "product_version": "$version"},
  "info": {
    "0409": {
      "FileVersion": "$version",
      "ProductVersion": "$version",
      "FileDescription": "Rho",
      "ProductName": "Rho"
    }
  }
}
JSON
cd "$here/../.."
for arch in amd64 arm64; do
  CGO_ENABLED=0 go tool wails3 generate syso -arch "$arch" \
    -icon build/windows/icon.ico \
    -manifest build/windows/rho-gui.exe.manifest \
    -info "$info" \
    -out "rsrc_windows_$arch.syso"
done
