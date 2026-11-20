#!/bin/sh
# Builds Rho.app (Apple silicon) containing rho-gui and rho from the
# darwin release archive, signs it and, when the Apple credentials are
# set, notarizes it. Runs on macOS (it needs codesign and ditto).
#
# Usage: sh bundle.sh VERSION DIR
#   DIR holds rho-gui_VERSION_darwin_arm64.tar.gz; Rho_VERSION_macos.zip
#   is written there.
#
# Signing uses a Developer ID Application certificate when MACOS_SIGN_P12
# (the .p12 file, base64) and MACOS_SIGN_PASSWORD are set, and an ad-hoc
# signature otherwise. Notarizing needs MACOS_NOTARY_KEY (the App Store
# Connect API .p8 key, base64), MACOS_NOTARY_KEY_ID and
# MACOS_NOTARY_ISSUER_ID as well.
set -eu
version=$1
dir=$(cd "$2" && pwd)
here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
keychain=
cleanup() {
  if [ -n "$keychain" ]; then
    # shellcheck disable=SC2046
    security list-keychains -d user -s $(cat "$work/keychains")
    security delete-keychain "$keychain"
  fi
  rm -rf "$work"
}
trap cleanup EXIT

mkdir "$work/arm64"
tar -xzf "$dir/rho-gui_${version}_darwin_arm64.tar.gz" -C "$work/arm64"

app="$work/Rho.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
for bin in rho-gui rho; do
  cp "$work/arm64/$bin" "$app/Contents/MacOS/$bin"
done
cp "$here/icons.icns" "$app/Contents/Resources/icons.icns"
sed "s/@VERSION@/$version/g" "$here/Info.plist" > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist"

identity=-
sign_flags=
if [ -n "${MACOS_SIGN_P12:-}" ]; then
  security list-keychains -d user | tr -d '"' > "$work/keychains"
  password=$(uuidgen)
  security create-keychain -p "$password" "$work/sign.keychain-db"
  keychain="$work/sign.keychain-db"
  security set-keychain-settings -lut 3600 "$keychain"
  security unlock-keychain -p "$password" "$keychain"
  printf '%s' "$MACOS_SIGN_P12" | base64 --decode > "$work/cert.p12"
  security import "$work/cert.p12" -k "$keychain" -P "${MACOS_SIGN_PASSWORD:-}" -T /usr/bin/codesign
  security set-key-partition-list -S apple-tool:,apple: -s -k "$password" "$keychain" > /dev/null
  # shellcheck disable=SC2046
  security list-keychains -d user -s "$keychain" $(cat "$work/keychains")
  identity=$(security find-identity -v -p codesigning "$keychain" | awk -F'"' '/Developer ID Application/ { print $2; exit }')
  if [ -z "$identity" ]; then
    echo "no Developer ID Application certificate in MACOS_SIGN_P12" >&2
    exit 1
  fi
  sign_flags="--options runtime --timestamp"
fi
# Sign the inner programs first, then the bundle.
# shellcheck disable=SC2086
codesign --force $sign_flags --sign "$identity" "$app/Contents/MacOS/rho"
# shellcheck disable=SC2086
codesign --force $sign_flags --sign "$identity" "$app/Contents/MacOS/rho-gui"
# shellcheck disable=SC2086
codesign --force $sign_flags --sign "$identity" "$app"
codesign --verify --strict --verbose=2 "$app"

zip="$dir/Rho_${version}_macos.zip"
rm -f "$zip"
ditto -c -k --keepParent "$app" "$zip"

if [ "$identity" != "-" ] && [ -n "${MACOS_NOTARY_KEY:-}" ]; then
  printf '%s' "$MACOS_NOTARY_KEY" | base64 --decode > "$work/notary.p8"
  xcrun notarytool submit "$zip" --wait \
    --key "$work/notary.p8" --key-id "$MACOS_NOTARY_KEY_ID" --issuer "$MACOS_NOTARY_ISSUER_ID"
  xcrun stapler staple "$app"
  rm -f "$zip"
  ditto -c -k --keepParent "$app" "$zip"
fi
echo "wrote $zip"
