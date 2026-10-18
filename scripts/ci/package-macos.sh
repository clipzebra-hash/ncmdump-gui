#!/usr/bin/env bash
set -euo pipefail
app=build/bin/ncmdump-gui.app
binary="$app/Contents/MacOS/ncmdump-gui"
test "$(lipo -archs "$binary")" = x86_64
file "$binary"
xcrun vtool -show-build "$binary"
# Inspect the actual Mach-O target, not just the plist declaration.
minimum=$(xcrun vtool -show-build "$binary" | awk '$1 == "minos" {print $2}')
test "$minimum" = 13.0
plist_min=$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$app/Contents/Info.plist")
test "$plist_min" = 13.0
codesign --force --deep --sign - "$app"
codesign --verify --deep --strict --verbose=2 "$app"
mkdir -p ci-package
# ditto preserves app permissions and bundle metadata inside the zip.
ditto -c -k --sequesterRsrc --keepParent "$app" ci-package/ncmdump-gui-macos-intel.zip
cp LICENSE ci-package/LICENSE
cp third_party/ncmdump-go/LICENSE ci-package/LICENSE-ncmdump-go
cp frontend/src/assets/fonts/OFL.txt ci-package/LICENSE-font-OFL.txt
{
  echo 'Community CI build; unsigned by Apple Developer ID and not notarized.'
  echo 'Target: macOS Intel x86_64, minimum deployment target 13.0.'
  echo 'CI compilation and tests do not prove Ventura 13.7.8 GUI/SMB acceptance.'
  echo 'Windows/Linux and real-music/SMB regression are not verified here.'
  echo "Source: https://github.com/clipzebra-hash/ncmdump-gui/commit/$(git rev-parse HEAD)"
  go version
  node --version
  pnpm --version
} > ci-package/BUILD-INFO.txt
(cd ci-package && shasum -a 256 ncmdump-gui-macos-intel.zip > SHA256SUMS.txt)
