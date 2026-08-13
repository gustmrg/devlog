#!/bin/sh

set -eu

repository="gustmrg/devlog"
release_api="https://api.github.com/repos/$repository/releases/latest"
install_dir=/usr/local/bin
binary_path="$install_dir/devlog"
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/devlog-install.XXXXXX")

cleanup() {
    rm -rf "$temporary_dir"
}

trap cleanup EXIT

if [ -z "${HOME:-}" ]; then
    printf '%s\n' "Error: HOME is not set; cannot determine the DevLog data directory." >&2
    exit 1
fi

for command_name in curl tar install; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        printf '%s\n' "Error: $command_name is required to install DevLog." >&2
        exit 1
    fi
done

case "$(uname -s)" in
    Darwin)
        operating_system=darwin
        ;;
    Linux)
        operating_system=linux
        ;;
    *)
        printf '%s\n' "Error: this installer supports macOS and Linux. Download the Windows ZIP from the releases page." >&2
        exit 1
        ;;
esac

case "$(uname -m)" in
    arm64|aarch64)
        architecture=arm64
        ;;
    amd64|x86_64)
        architecture=amd64
        ;;
    *)
        printf '%s\n' "Error: unsupported CPU architecture: $(uname -m)" >&2
        exit 1
        ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
    sha256_command="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    sha256_command="shasum"
else
    printf '%s\n' "Error: sha256sum or shasum is required to verify the release." >&2
    exit 1
fi

release_json="$temporary_dir/release.json"
curl --fail --silent --show-error --location --retry 3 "$release_api" -o "$release_json"

release_tag=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$release_json" | head -n 1)
if [ -z "$release_tag" ]; then
    printf '%s\n' "Error: could not determine the latest DevLog release." >&2
    exit 1
fi

release_version=${release_tag#v}
archive_name="devlog_${release_version}_${operating_system}_${architecture}.tar.gz"
archive_path="$temporary_dir/$archive_name"
checksums_path="$temporary_dir/checksums.txt"
download_base="https://github.com/$repository/releases/download/$release_tag"

printf 'Downloading DevLog %s for %s/%s\n' "$release_tag" "$operating_system" "$architecture"
curl --fail --silent --show-error --location --retry 3 \
    "$download_base/$archive_name" -o "$archive_path"
curl --fail --silent --show-error --location --retry 3 \
    "$download_base/checksums.txt" -o "$checksums_path"

expected_checksum=$(awk -v archive_name="$archive_name" '$2 == archive_name { print $1; exit }' "$checksums_path")
if [ -z "$expected_checksum" ]; then
    printf '%s\n' "Error: no checksum was published for $archive_name." >&2
    exit 1
fi

if [ "$sha256_command" = "sha256sum" ]; then
    actual_checksum=$(sha256sum "$archive_path" | awk '{ print $1 }')
else
    actual_checksum=$(shasum -a 256 "$archive_path" | awk '{ print $1 }')
fi

if [ "$actual_checksum" != "$expected_checksum" ]; then
    printf '%s\n' "Error: checksum verification failed for $archive_name." >&2
    exit 1
fi

mkdir "$temporary_dir/extracted"
tar -xzf "$archive_path" -C "$temporary_dir/extracted"
downloaded_binary="$temporary_dir/extracted/devlog"

if [ ! -f "$downloaded_binary" ]; then
    printf '%s\n' "Error: the release archive does not contain the devlog binary." >&2
    exit 1
fi

if [ -w "$install_dir" ]; then
    install -m 0755 "$downloaded_binary" "$binary_path"
elif command -v sudo >/dev/null 2>&1; then
    sudo install -m 0755 "$downloaded_binary" "$binary_path"
else
    printf '%s\n' "Error: cannot write to $install_dir; run this script with an account that has sudo access." >&2
    exit 1
fi

config_path="$HOME/.devlog/config.json"
if [ ! -f "$config_path" ]; then
    "$binary_path" init
else
    printf 'Keeping existing configuration at %s\n' "$config_path"
fi

printf 'Installed DevLog %s at %s\n' "$release_tag" "$binary_path"
