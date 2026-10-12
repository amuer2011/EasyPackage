#!/bin/sh
set -eu

if [ "$#" -ne 4 ]; then
    echo "Usage: $0 <binary> <release-tag> <platform> <output-directory>" >&2
    exit 2
fi

binary=$1
release_tag=$2
platform=$3
output_directory=$4
case "$platform" in
    macos-amd64|macos-arm64) ;;
    *) echo "Unsupported macOS platform: $platform" >&2; exit 2 ;;
esac

script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_directory=$(dirname "$script_directory")
staging_directory=$(mktemp -d)
trap 'rm -rf "$staging_directory"' EXIT HUP INT TERM

app="$staging_directory/EasyPackage.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$output_directory"
cp "$binary" "$app/Contents/MacOS/EasyPackage"
chmod 755 "$app/Contents/MacOS/EasyPackage"
cp "$project_directory/assets/macos/easypackage.icns" "$app/Contents/Resources/EasyPackage.icns"

version=${release_tag#v}
cat > "$app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>io.github.amuer2011.EasyPackage</string>
<key>CFBundleName</key><string>EasyPackage</string>
<key>CFBundleDisplayName</key><string>EasyPackage</string>
<key>CFBundleExecutable</key><string>EasyPackage</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleIconFile</key><string>EasyPackage.icns</string>
<key>CFBundleShortVersionString</key><string>$version</string>
<key>CFBundleVersion</key><string>$version</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>
EOF

codesign --force --sign - "$app"
ln -s /Applications "$staging_directory/Applications"
hdiutil create -srcfolder "$staging_directory" -volname EasyPackage -format UDZO -ov "$output_directory/EasyPackage-$release_tag-$platform.dmg"
