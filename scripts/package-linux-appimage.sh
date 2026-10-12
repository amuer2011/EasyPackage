#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
    echo "Usage: $0 <binary> <release-tag> <output-directory>" >&2
    exit 2
fi

binary=$1
release_tag=$2
output_directory=$3
script_directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_directory=$(dirname "$script_directory")
staging_directory=$(mktemp -d)
trap 'rm -rf "$staging_directory"' EXIT HUP INT TERM

appdir="$staging_directory/EasyPackage.AppDir"
mkdir -p "$appdir/usr/bin" "$output_directory"
cp "$binary" "$appdir/usr/bin/EasyPackage"
chmod 755 "$appdir/usr/bin/EasyPackage"
cp "$project_directory/assets/linux/easypackage.png" "$appdir/easypackage.png"

cat > "$appdir/AppRun" <<'EOF'
#!/bin/sh
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$HERE/usr/bin/EasyPackage" "$@"
EOF
chmod 755 "$appdir/AppRun"

cat > "$appdir/easypackage.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=EasyPackage
Exec=EasyPackage
Icon=easypackage
Categories=Utility;Archiving;
Terminal=false
EOF

tool="$staging_directory/appimagetool-x86_64.AppImage"
curl -fsSL --retry 3 -o "$tool" 'https://github.com/AppImage/appimagetool/releases/download/1.9.1/appimagetool-x86_64.AppImage'
echo "ed4ce84f0d9caff66f50bcca6ff6f35aae54ce8135408b3fa33abfc3cb384eb0  $tool" | sha256sum -c -
chmod 755 "$tool"

ARCH=x86_64 "$tool" --appimage-extract-and-run "$appdir" "$output_directory/EasyPackage-$release_tag-linux-amd64.AppImage"
