#!/bin/sh
set -eu

binary=$1
icon=$2

if [ "${GDK_BACKEND:-}" = x11 ] || { [ "${XDG_SESSION_TYPE:-}" != wayland ] && [ -z "${WAYLAND_DISPLAY:-}" ]; }; then
    exec "$binary"
fi

# GTK3 advertises the program name as the Wayland app ID. The compositor
# resolves the icon from the desktop entry with that ID.
applications=${XDG_DATA_HOME:-"$HOME/.local/share"}/applications
if ! mkdir -p -- "$applications" || ! entry_base=$(mktemp -- "$applications/.playnux-dev-XXXXXXXX"); then
    exec "$binary"
fi
app_id=${entry_base##*/.}
entry="$applications/$app_id.desktop"
trap 'rm -f -- "$entry_base" "$entry"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# The desktop parser unescapes the value before parsing the quoted command.
command=$(printf '%s' "$binary" | sed -e 's/[\\"\$`]/\\\\\\\\&/g' -e 's/%/%%/g')
icon_path=$(printf '%s' "$icon" | sed 's/\\/\\\\/g')
{
    printf '[Desktop Entry]\nType=Application\nName=Playnux\nNoDisplay=true\n'
    printf 'Exec="%s"\nIcon=%s\nStartupWMClass=%s\n' "$command" "$icon_path" "$app_id"
} > "$entry_base"
mv -- "$entry_base" "$entry"

case ":${XDG_CURRENT_DESKTOP:-}:" in
    *:KDE:*)
        if command -v kbuildsycoca6 >/dev/null 2>&1; then
            kbuildsycoca6 >/dev/null 2>&1 || :
        fi
        ;;
esac

PLAYNUX_RUN_DESKTOP_ID="$app_id" "$binary"
