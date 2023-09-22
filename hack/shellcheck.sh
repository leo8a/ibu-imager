#!/bin/bash

shellcheck_version="v0.7.2"
shellcheck_binary="$(go env GOPATH)/bin/shellcheck"


if [ ! -f "${shellcheck_binary}" ]; then
    echo "Downloading shellcheck tool..."
    download_url="https://github.com/koalaman/shellcheck/releases/download/$shellcheck_version/shellcheck-$shellcheck_version.linux.x86_64.tar.xz"

    # Create a temporary directory for extraction
    temp_dir="$(mktemp -d)"

    # Download and extract ShellCheck
    wget -qO- "$download_url" | tar -xJ -C "$temp_dir" --strip=1 "shellcheck-$shellcheck_version/shellcheck"

    # Move shellcheck binary to the desired location
    mv "$temp_dir/shellcheck" "$shellcheck_binary"

    # Clean up temporary directory
    rm -r "$temp_dir"
fi


# Find and check shell script files with ShellCheck
find . -name '*.sh' -not -path './vendor/*' -not -path './git/*' \
    -not -path './bin/*' -not -path './testbin/*' -print0 \
    | xargs -0 --no-run-if-empty "${shellcheck_binary}" -x
