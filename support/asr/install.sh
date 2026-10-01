#!/usr/bin/env bash
# Install the speech recognition runtime (sherpa-onnx) and the Vosk small-ru model,
# then enable nanokvm-asr. Both live outside /kvmapp and survive application updates.
#
# Usage: /kvmapp/asr/install.sh
# Mirrors: PIP_INDEX_URL for pip, ASR_MODEL_URL for the model files.
set -euo pipefail

venv=/root/npu/venv
model_dir=/root/npu/vosk
model_url="${ASR_MODEL_URL:-https://huggingface.co/alphacep/vosk-model-small-ru/resolve/main}"
sherpa_version=1.13.8

model_files=(
    "75653125bf3621d86a2b140ffd84a1c3b7610fc8cd49f2b6d70cf5786be0d085 am/encoder.int8.onnx"
    "8537bd782f59c94b509ea501976942563e61d437373917a7736252b869e3bb2d am/decoder.onnx"
    "a93e0afc2c82e94d366157b4a8b5f4cba6973ddea8f36ac94010b430be7823f1 am/joiner.int8.onnx"
    "93bbbc0bae6b78c0bbb743d4aa9fded3bb5ff3aac5f0200e3a769a5a05e0fdf6 lang/tokens.txt"
)

[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }

if [[ ! -x "${venv}/bin/python" ]]; then
    echo "Creating ${venv}"
    # numpy and onnxruntime come from the system Python of the NanoKVM image.
    python3 -m venv --system-site-packages "$venv"
fi
if "${venv}/bin/python" -c 'import numpy, sherpa_onnx' 2>/dev/null; then
    echo "sherpa-onnx is already installed"
else
    "${venv}/bin/pip" install --disable-pip-version-check "sherpa-onnx==${sherpa_version}" numpy
fi

for entry in "${model_files[@]}"; do
    sum=${entry%% *}
    file=${entry#* }
    target="${model_dir}/${file}"
    if [[ -f "$target" ]] && echo "${sum}  ${target}" | sha256sum -c --status; then
        continue
    fi
    echo "Downloading ${file}"
    mkdir -p "$(dirname "$target")"
    curl -fL --retry 3 -o "${target}.part" "${model_url}/${file}"
    echo "${sum}  ${target}.part" | sha256sum -c --status || {
        rm -f "${target}.part"
        echo "checksum mismatch: ${file}" >&2
        exit 1
    }
    mv "${target}.part" "$target"
done

systemctl daemon-reload
systemctl enable nanokvm-asr.service
systemctl restart nanokvm-asr.service
echo "nanokvm-asr is $(systemctl is-active nanokvm-asr.service); the model takes a few seconds to load"
