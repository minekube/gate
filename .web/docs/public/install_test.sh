#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../../.." && pwd)

fail() {
    echo "FAIL: $1" >&2
    exit 1
}

run_test() {
    (
        "$@"
    )
}

write_fake_uname() {
    dir="$1"
    cat >"$dir/uname" <<'EOF'
#!/bin/sh
case "$1" in
    -s) echo Linux ;;
    -m) echo x86_64 ;;
    *) exit 1 ;;
esac
EOF
    chmod +x "$dir/uname"
}

write_fake_sha256sum() {
    dir="$1"
    cat >"$dir/sha256sum" <<'EOF'
#!/bin/sh
printf 'test-sha  %s\n' "$1"
EOF
    chmod +x "$dir/sha256sum"
}

link_required_tools_without_curl() {
    dir="$1"
    for cmd in awk basename cat chmod cp df dirname grep mkdir mktemp mv rm sed; do
        tool_path=$(command -v "$cmd") || fail "missing required test tool: $cmd"
        ln -s "$tool_path" "$dir/$cmd"
    done
}

test_skips_release_without_assets() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
log="${CURL_LOG:?}"
echo "$*" >> "$log"

output_arg() {
    previous=
    for arg in "$@"; do
        if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
            echo "$arg"
            return 0
        fi
        previous="$arg"
    done
    return 1
}

case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        cat <<'JSON'
[
  {"tag_name": "v2.0.0", "prerelease": false},
  {"tag_name": "v1.0.0", "prerelease": false}
]
JSON
        exit 0
        ;;
    *"v2.0.0/gate_2.0.0_linux_amd64"*)
        exit 22
        ;;
    *"v2.0.0/checksums.txt"*)
        exit 22
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        out=$(output_arg "$@")
        cat >"$out" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
        exit 0
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        out=$(output_arg "$@")
        printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$out"
        exit 0
        ;;
esac

echo "unexpected curl: $*" >&2
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    CURL_LOG="$tmp/curl.log" \
    GATE_INSTALL_DIR="$tmp/install" \
    GATE_INSTALLER_ASSUME_GLIBC=1 \
    PATH="$tmp/bin:$PATH" \
        bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"

    "$tmp/install/gate" --version | grep -q "gate version test" || fail "installed binary came from the wrong release"
    grep -q "v2.0.0/gate_2.0.0_linux_amd64" "$tmp/curl.log" || fail "newest asset was not probed"
    grep -q "v1.0.0/gate_1.0.0_linux_amd64" "$tmp/curl.log" || fail "fallback asset was not downloaded"
}

test_download_uses_fail_flag() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        case "$*" in
            *"-f"*|*"--fail"*) ;;
            *) exit 22 ;;
        esac
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    GATE_INSTALL_DIR="$tmp/install" \
    GATE_INSTALLER_ASSUME_GLIBC=1 \
    PATH="$tmp/bin:$PATH" \
        bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"
}

test_verifies_installed_binary() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"
    cat >"$tmp/install/gate" <<'GATE'
#!/bin/sh
echo "old gate"
GATE
    chmod +x "$tmp/install/gate"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
exit 42
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    if GATE_INSTALL_DIR="$tmp/install" \
        GATE_INSTALLER_ASSUME_GLIBC=1 \
        PATH="$tmp/bin:$PATH" \
        bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out" 2>&1; then
        fail "installer succeeded even though the installed binary does not run"
    fi

    grep -q "Installed Gate binary failed to run" "$tmp/install.out" || fail "missing executable verification error"
    "$tmp/install/gate" --version | grep -q "old gate" || fail "failed staged binary replaced existing install"
}

test_linux_musl_selects_musl_asset() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
log="${CURL_LOG:?}"
echo "$*" >> "$log"

case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64_musl"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                printf 'test-sha  gate_1.0.0_linux_amd64_musl\n' > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 22
EOF
    chmod +x "$tmp/bin/curl"

    CURL_LOG="$tmp/curl.log" \
    GATE_INSTALL_DIR="$tmp/install" \
    GATE_INSTALLER_LINUX_LIBC=musl \
    PATH="$tmp/bin:$PATH" \
        bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"

    grep -q "gate_1.0.0_linux_amd64_musl" "$tmp/curl.log" || fail "musl asset was not selected"
    if grep -q "gate_1.0.0_linux_amd64 " "$tmp/curl.log"; then
        fail "glibc asset was selected for musl Linux"
    fi
}

test_checksum_matches_exact_filename() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                {
                    printf 'wrong-sha  gate_1.0.0_linux_amd64_musl\n'
                    printf 'test-sha  gate_1.0.0_linux_amd64\n'
                } > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    GATE_INSTALL_DIR="$tmp/install" \
    GATE_INSTALLER_ASSUME_GLIBC=1 \
    PATH="$tmp/bin:$PATH" \
        bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"
}

test_wget_only_fetches_release_metadata() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    link_required_tools_without_curl "$tmp/bin"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"
    bash_path=$(command -v bash) || fail "missing bash"

    cat >"$tmp/bin/wget" <<'EOF'
#!/bin/sh
log="${WGET_LOG:?}"
echo "$*" >> "$log"

case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"--spider"*"v1.0.0/gate_1.0.0_linux_amd64"*)
        exit 0
        ;;
    *"--spider"*"v1.0.0/checksums.txt"*)
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        out=
        previous=
        for arg in "$@"; do
            if [ "$previous" = "-O" ]; then
                out="$arg"
                break
            fi
            previous="$arg"
        done
        [ -n "$out" ] || exit 2
        cat >"$out" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
        exit 0
        ;;
    *"v1.0.0/checksums.txt"*)
        out=
        previous=
        for arg in "$@"; do
            if [ "$previous" = "-O" ]; then
                out="$arg"
                break
            fi
            previous="$arg"
        done
        [ -n "$out" ] || exit 2
        printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$out"
        exit 0
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/wget"

    WGET_LOG="$tmp/wget.log" \
    GATE_INSTALL_DIR="$tmp/install" \
    GATE_INSTALLER_ASSUME_GLIBC=1 \
    PATH="$tmp/bin" \
        "$bash_path" "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"

    grep -q "api.github.com/repos/minekube/gate/releases" "$tmp/wget.log" || fail "release metadata was not fetched with wget"
    "$tmp/install/gate" --version | grep -q "gate version test" || fail "wget-only install did not produce a runnable binary"
}

test_prereleases_are_skipped() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
log="${CURL_LOG:?}"
echo "$*" >> "$log"

case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        cat <<'JSON'
[
  {"tag_name": "v2.0.0-rc.1", "prerelease": true},
  {"tag_name": "v1.0.0", "prerelease": false}
]
JSON
        exit 0
        ;;
    *"v2.0.0-rc.1"*)
        exit 2
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    CURL_LOG="$tmp/curl.log" \
    GATE_INSTALL_DIR="$tmp/install" \
    GATE_INSTALLER_ASSUME_GLIBC=1 \
    PATH="$tmp/bin:$PATH" \
        bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"

    if grep -q "v2.0.0-rc.1" "$tmp/curl.log"; then
        fail "prerelease asset was probed"
    fi
    grep -q "v1.0.0/gate_1.0.0_linux_amd64" "$tmp/curl.log" || fail "stable release was not selected"
}

test_missing_checksum_tool_fails_closed() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    link_required_tools_without_curl "$tmp/bin"
    write_fake_uname "$tmp/bin"
    bash_path=$(command -v bash) || fail "missing bash"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    if GATE_INSTALL_DIR="$tmp/install" \
        GATE_INSTALLER_ASSUME_GLIBC=1 \
        PATH="$tmp/bin" \
        "$bash_path" "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out" 2>&1; then
        fail "installer succeeded without a checksum tool"
    fi

    grep -q "No SHA256 checksum tool found" "$tmp/install.out" || fail "missing checksum tool error"
    [ ! -e "$tmp/install/gate" ] || fail "unverified binary was installed"
}

test_replacement_is_staged_in_install_dir() {
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    mkdir -p "$tmp/bin" "$tmp/install"
    write_fake_uname "$tmp/bin"
    write_fake_sha256sum "$tmp/bin"
    real_cp=$(command -v cp) || fail "missing cp"

    cat >"$tmp/bin/cp" <<EOF
#!/bin/sh
echo "\$2" >> "\${CP_LOG:?}"
exec "$real_cp" "\$@"
EOF
    chmod +x "$tmp/bin/cp"

    cat >"$tmp/bin/curl" <<'EOF'
#!/bin/sh
case "$*" in
    *"https://api.github.com/repos/minekube/gate/releases?per_page=10"*)
        printf '[{"tag_name":"v1.0.0","prerelease":false}]\n'
        exit 0
        ;;
    *"v1.0.0/gate_1.0.0_linux_amd64"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                cat >"$arg" <<'GATE'
#!/bin/sh
echo "gate version test"
GATE
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
    *"v1.0.0/checksums.txt"*)
        if [ "$#" -gt 0 ] && [ "$1" = "-fsSIL" ]; then
            exit 0
        fi
        previous=
        for arg in "$@"; do
            if [ "$previous" = "--output" ] || [ "$previous" = "-o" ]; then
                printf 'test-sha  gate_1.0.0_linux_amd64\n' > "$arg"
                exit 0
            fi
            previous="$arg"
        done
        exit 2
        ;;
esac
exit 2
EOF
    chmod +x "$tmp/bin/curl"

    (
        umask 077
        CP_LOG="$tmp/cp.log" \
        GATE_INSTALL_DIR="$tmp/install" \
        GATE_INSTALLER_ASSUME_GLIBC=1 \
        PATH="$tmp/bin:$PATH" \
            bash "$ROOT_DIR/.web/docs/public/install" >"$tmp/install.out"
    )

    grep -q "^$tmp/install/gate.tmp\\." "$tmp/cp.log" || fail "binary was not staged inside install dir"
    "$tmp/install/gate" --version | grep -q "gate version test" || fail "staged install did not produce a runnable binary"
    mode=$(ls -l "$tmp/install/gate" | awk '{print $1}')
    case "$mode" in
        -rwxr-xr-x*) ;;
        *) fail "installed binary mode was $mode, want -rwxr-xr-x" ;;
    esac
}

run_test test_skips_release_without_assets
run_test test_download_uses_fail_flag
run_test test_verifies_installed_binary
run_test test_linux_musl_selects_musl_asset
run_test test_checksum_matches_exact_filename
run_test test_wget_only_fetches_release_metadata
run_test test_prereleases_are_skipped
run_test test_missing_checksum_tool_fails_closed
run_test test_replacement_is_staged_in_install_dir
