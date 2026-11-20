#!/bin/sh
# Regenerates the icons from appicon.svg: appicon.png (embedded in rho-gui),
# windows/icon.ico and darwin/icons.icns. Needs rsvg-convert (librsvg).
set -eu
cd "$(dirname "$0")/.."
rsvg-convert -w 1024 -h 1024 build/appicon.svg -o build/appicon.png
CGO_ENABLED=0 go tool wails3 generate icons -input build/appicon.png \
  -windowsfilename build/windows/icon.ico -macfilename build/darwin/icons.icns
