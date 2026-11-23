#!/bin/sh
set -eu

prefix=${PLAYNUX_INSTALL_PREFIX:-"$HOME/.local"}
share=${XDG_DATA_HOME:-"$prefix/share"}
case "$prefix:$share" in
    /*:/*) ;;
    *) printf '%s\n' 'The install prefix and XDG_DATA_HOME must be absolute paths.' >&2; exit 1 ;;
esac

app="$prefix/opt/playnux"
binary="$app/playnux"
link="$prefix/bin/playnux"
desktop="$share/applications/playnux.desktop"
icon="$share/icons/hicolor/512x512/apps/playnux.png"

if [ "${1:-}" = --uninstall ]; then
    if [ ! -f "$app/.managed" ]; then
        printf '%s\n' 'No managed Playnux installation found.' >&2
        exit 1
    fi
    if [ -L "$link" ] && [ "$(readlink "$link")" = "$binary" ]; then
        rm "$link"
    fi
    rm -f "$binary" "$binary.previous" "$app/.managed" "$desktop" "$icon"
    rmdir "$app" 2>/dev/null || :
    printf '%s\n' 'Playnux removed. Your library and settings are preserved.'
    exit 0
fi

if [ "$#" -ne 0 ]; then
    printf '%s\n' 'Usage: sh install.sh [--uninstall]' >&2
    exit 2
fi

if [ -e "$link" ] || [ -L "$link" ]; then
    if [ ! -L "$link" ] || [ "$(readlink "$link")" != "$binary" ]; then
        printf 'An unrelated file already exists at %s\n' "$link" >&2
        exit 1
    fi
fi
if [ ! -f "$app/.managed" ] && { [ -e "$binary" ] || [ -e "$desktop" ] || [ -e "$icon" ]; }; then
    printf '%s\n' 'A Playnux file already exists outside this installer’s management.' >&2
    exit 1
fi

source_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
version=$("$source_dir/playnux" --version)
case "$version" in
    'Playnux '*) ;;
    *) printf '%s\n' 'The Playnux binary could not report its version.' >&2; exit 1 ;;
esac

mkdir -p "$app" "$prefix/bin" "$(dirname -- "$desktop")" "$(dirname -- "$icon")"
staging=$(mktemp -d "$app/.install.XXXXXX")
trap 'rm -f "$staging/playnux" "$staging/playnux.desktop"; rmdir "$staging"' EXIT

install -m 755 "$source_dir/playnux" "$staging/playnux"
# The desktop field parser consumes one level of escaping before the command parser.
exec_path=$(printf '%s' "$link" | sed -e 's/[\\"\$`]/\\\\\\\\&/g' -e 's/%/%%/g')
while IFS= read -r line; do
    case "$line" in
        Exec=*) printf 'Exec="%s"\n' "$exec_path" ;;
        *) printf '%s\n' "$line" ;;
    esac
done < "$source_dir/playnux.desktop" > "$staging/playnux.desktop"
touch "$app/.managed"
mv -f "$staging/playnux" "$binary"
[ -L "$link" ] || ln -s "$binary" "$link"
install -m 644 "$staging/playnux.desktop" "$desktop"
install -m 644 "$source_dir/playnux.png" "$icon"

printf '%s installed at %s\n' "$version" "$link"
case ":$PATH:" in
    *":$prefix/bin:"*) ;;
    *) printf '%s\n' 'Open Playnux from your application menu or add the bin directory above to your PATH.' ;;
esac
