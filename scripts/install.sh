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

fail() {
    printf 'Error: %s\n' "$1" >&2
    exit 1
}

for command_name in curl tar install; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        fail "missing required command '$command_name'. Install it and try again."
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
        fail "unsupported operating system '$(uname -s)'. Use macOS or Linux, or download another build from https://github.com/$repository/releases."
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
        fail "unsupported CPU architecture '$(uname -m)'. Download a compatible build from https://github.com/$repository/releases."
        ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
    sha256_command="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    sha256_command="shasum"
else
    fail "checksum verification requires 'sha256sum' or 'shasum'. Install one and try again."
fi

release_json="$temporary_dir/release.json"
if ! curl --fail --silent --show-error --location --retry 3 "$release_api" -o "$release_json"; then
    fail "could not fetch the latest release information. Check your connection and try again."
fi

release_tag=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$release_json" | head -n 1)
if [ -z "$release_tag" ]; then
    fail "the GitHub response did not identify a latest DevLog release. Try again later."
fi

release_version=${release_tag#v}
archive_name="devlog_${release_version}_${operating_system}_${architecture}.tar.gz"
archive_path="$temporary_dir/$archive_name"
checksums_path="$temporary_dir/checksums.txt"
download_base="https://github.com/$repository/releases/download/$release_tag"

printf 'Downloading DevLog %s for %s/%s\n' "$release_tag" "$operating_system" "$architecture"
if ! curl --fail --silent --show-error --location --retry 3 \
    "$download_base/$archive_name" -o "$archive_path"; then
    fail "could not download $archive_name from release $release_tag."
fi
if ! curl --fail --silent --show-error --location --retry 3 \
    "$download_base/checksums.txt" -o "$checksums_path"; then
    fail "could not download checksums for release $release_tag."
fi

expected_checksum=$(awk -v archive_name="$archive_name" '$2 == archive_name { print $1; exit }' "$checksums_path")
if [ -z "$expected_checksum" ]; then
    fail "release $release_tag does not include a checksum for $archive_name; installation was stopped."
fi

if [ "$sha256_command" = "sha256sum" ]; then
    actual_checksum=$(sha256sum "$archive_path" | awk '{ print $1 }')
else
    actual_checksum=$(shasum -a 256 "$archive_path" | awk '{ print $1 }')
fi

if [ "$actual_checksum" != "$expected_checksum" ]; then
    fail "checksum verification failed for $archive_name; the downloaded file was not installed."
fi

mkdir "$temporary_dir/extracted"
if ! tar -xzf "$archive_path" -C "$temporary_dir/extracted"; then
    fail "could not extract $archive_name; the downloaded archive may be invalid."
fi
downloaded_binary="$temporary_dir/extracted/devlog"

if [ ! -f "$downloaded_binary" ]; then
    fail "release archive $archive_name does not contain the devlog binary."
fi

if [ -w "$install_dir" ]; then
    if ! install -m 0755 "$downloaded_binary" "$binary_path"; then
        fail "could not install DevLog at $binary_path. Check the directory permissions and try again."
    fi
elif command -v sudo >/dev/null 2>&1; then
    if ! sudo install -m 0755 "$downloaded_binary" "$binary_path"; then
        fail "could not install DevLog at $binary_path with sudo. Check your sudo access and try again."
    fi
else
    fail "cannot write to $install_dir and sudo is unavailable. Install the binary manually or use an account with access."
fi

printf 'Installed DevLog %s at %s\n' "$release_tag" "$binary_path"

if [ -z "${HOME:-}" ] || [ ! -f "$HOME/.devlog/config.json" ]; then
    printf '%s\n' "Next step: run 'devlog init' to create the DevLog configuration."
fi
