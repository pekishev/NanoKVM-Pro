#!/usr/bin/env bash
# Build an official-format application package (nanokvm_pro_<version>.tar.gz and
# nanokvm_pro_latest.json) from the fork server binary and web UI.
#
# The closed components (libkvm, pikvm, kvmcomm) come from the official package
# named by [official] base_version in config.ini. Every DEB is rebuilt with the
# fork version because the in-device updater requires matching DEB versions.
# The nanokvm DEB also carries the speech recognition service from support/asr.
#
# Usage:
#   repack_official.sh --version 1.2.16-fork.1 --server NanoKVM-Server --web web/dist \
#     [--out dist] [--base-archive nanokvm_pro_1.2.15.tar.gz]
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
config="${script_dir}/config.ini"
asr_dir="${script_dir}/../asr"
apps=(nanokvmpro pikvm kvmcomm)

version=""
server=""
web=""
out="dist"
base_archive=""

die() {
    echo "error: $*" >&2
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
    --version) version="${2#v}"; shift 2 ;;
    --server) server="$2"; shift 2 ;;
    --web) web="$2"; shift 2 ;;
    --out) out="$2"; shift 2 ;;
    --base-archive) base_archive="$2"; shift 2 ;;
    *) die "unknown argument: $1" ;;
    esac
done

[[ -n "$version" && -n "$server" && -n "$web" ]] || die "--version, --server and --web are required"
[[ "$version" =~ ^[0-9][A-Za-z0-9.+~-]{0,126}$ ]] || die "version is not usable for the updater and dpkg: $version"
[[ -f "$server" ]] || die "server binary not found: $server"
[[ -f "$web/index.html" ]] || die "web/index.html not found in: $web"
for tool in dpkg-deb tar curl python3; do
    command -v "$tool" >/dev/null || die "$tool is required"
done

base_version=$(python3 "${script_dir}/getconfig.py" "$config" official base_version)
base_url=$(python3 "${script_dir}/getconfig.py" "$config" official url)
[[ -n "$base_version" && -n "$base_url" ]] || die "[official] base_version and url must be set in config.ini"

sha512_b64() {
    python3 -c 'import base64, hashlib, sys; print(base64.b64encode(hashlib.sha512(open(sys.argv[1], "rb").read()).digest()).decode())' "$1"
}

json_field() {
    python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))[sys.argv[2]])' "$1" "$2"
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [[ -z "$base_archive" ]]; then
    base_archive="${work}/nanokvm_pro_${base_version}.tar.gz"
    echo "Downloading ${base_url}/nanokvm_pro_${base_version}.tar.gz"
    curl -fsSL --retry 3 -o "$base_archive" "${base_url}/nanokvm_pro_${base_version}.tar.gz"

    # The CDN manifest only describes its latest release, so the archive hash can
    # be pinned only while base_version is the current official release.
    manifest="${work}/official_latest.json"
    if curl -fsSL --retry 3 -o "$manifest" "${base_url}/nanokvm_pro_latest.json" &&
        [[ "$(json_field "$manifest" version)" == "$base_version" ]]; then
        [[ "$(sha512_b64 "$base_archive")" == "$(json_field "$manifest" sha512)" ]] ||
            die "official archive does not match the official manifest"
        echo "Official archive matches the official manifest"
    else
        echo "warning: official manifest is not for ${base_version}; relying on the per-DEB checksums" >&2
    fi
fi

base_root="nanokvm_pro_${base_version}"
tar -xzf "$base_archive" -C "$work"
[[ -d "${work}/${base_root}" ]] || die "base archive has no ${base_root}/ directory"

root="nanokvm_pro_${version}"
stage="${work}/out/${root}"
mkdir -p "$stage"

for app in "${apps[@]}"; do
    deb="${work}/${base_root}/${app}_${base_version}_arm64.deb"
    meta="${work}/${base_root}/${app}_${base_version}.json"
    [[ -f "$deb" && -f "$meta" ]] || die "base archive is missing ${app}"
    [[ "$(sha512_b64 "$deb")" == "$(json_field "$meta" sha512)" ]] || die "${app} DEB does not match its metadata"

    tree="${work}/tree-${app}"
    dpkg-deb -R "$deb" "$tree"
    sed -i "s/^Version: .*/Version: ${version}/" "${tree}/DEBIAN/control"

    if [[ "$app" == "nanokvmpro" ]]; then
        server_dir="${tree}/kvmapp/server"
        [[ -f "${server_dir}/NanoKVM-Server" && -d "${server_dir}/web" ]] || die "unexpected nanokvm DEB layout"
        install -m 0755 "$server" "${server_dir}/NanoKVM-Server"
        rm -rf "${server_dir}/web"
        mkdir -p "${server_dir}/web"
        cp -R "${web}/." "${server_dir}/web/"
        find "${server_dir}/web" -type d -exec chmod 0755 {} +
        find "${server_dir}/web" -type f -exec chmod 0644 {} +
        printf '%s\n' "$version" >"${tree}/kvmapp/version"

        install -D -m 0644 "${asr_dir}/asr_server.py" "${tree}/kvmapp/asr/asr_server.py"
        install -D -m 0755 "${asr_dir}/install.sh" "${tree}/kvmapp/asr/install.sh"
        install -D -m 0644 "${asr_dir}/nanokvm-asr.service" "${tree}/etc/systemd/system/nanokvm-asr.service"
        for script in postinst prerm; do
            # An official script that exits early would skip the appended ASR steps.
            ! grep -qE '^[[:space:]]*exit' "${tree}/DEBIAN/${script}" ||
                die "official ${script} exits early; update the ASR packaging"
            cat "${asr_dir}/${script}" >>"${tree}/DEBIAN/${script}"
        done
    fi

    out_deb="${stage}/${app}_${version}_arm64.deb"
    # xz keeps the DEBs readable by the device dpkg; newer hosts default to zstd.
    dpkg-deb --root-owner-group -Zxz -b "$tree" "$out_deb" >/dev/null
    for field in Version Architecture; do
        echo "${app}: ${field}=$(dpkg-deb -f "$out_deb" "$field")"
    done

    python3 - "$out_deb" "$version" "$(sha512_b64 "$out_deb")" >"${stage}/${app}_${version}.json" <<'EOF'
import json, os, sys
path, version, sha512 = sys.argv[1:4]
print(json.dumps({
    "version": version,
    "name": os.path.basename(path),
    "sha512": sha512,
    "size": os.path.getsize(path),
}, indent=2))
EOF
done

mkdir -p "$out"
archive="${out}/${root}.tar.gz"
tar --owner=0 --group=0 --numeric-owner -C "${work}/out" -czf "$archive" "$root"

python3 - "$archive" "$version" "$(sha512_b64 "$archive")" >"${out}/nanokvm_pro_latest.json" <<'EOF'
import json, os, sys
path, version, sha512 = sys.argv[1:4]
print(json.dumps({
    "version": version,
    "name": os.path.basename(path),
    "sha512": sha512,
    "size": os.path.getsize(path),
}, indent=2))
EOF

echo "Built ${archive} (base ${base_version})"
cat "${out}/nanokvm_pro_latest.json"
