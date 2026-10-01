#!/usr/bin/env python3
"""Local Russian speech recognition for NanoKVM (sherpa-onnx, Vosk small-ru Zipformer2).

POST /asr with a 16 kHz 16-bit mono WAV body, returns {"text": "..."}.
Listens on localhost only; NanoKVM-Server proxies /api/hid/speech here.
"""

import io
import json
import os
import threading
import time
import wave
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import numpy as np
import sherpa_onnx

MODEL_DIR = os.environ.get('ASR_MODEL_DIR', '/root/npu/vosk')
PORT = int(os.environ.get('ASR_PORT', '8766'))
MAX_BYTES = 60 * 16000 * 2 + 1024
DUMP_DIR = os.environ.get('ASR_DUMP_DIR', '')
DUMP_KEEP = 10

recognizer = None
lock = threading.Lock()


def load():
    return sherpa_onnx.OfflineRecognizer.from_transducer(
        encoder=f'{MODEL_DIR}/am/encoder.int8.onnx',
        decoder=f'{MODEL_DIR}/am/decoder.onnx',
        joiner=f'{MODEL_DIR}/am/joiner.int8.onnx',
        tokens=f'{MODEL_DIR}/lang/tokens.txt',
        num_threads=2,
        sample_rate=16000,
        decoding_method='greedy_search',
    )


def read_wav(data: bytes):
    with wave.open(io.BytesIO(data)) as w:
        if w.getsampwidth() != 2:
            raise ValueError('expect 16-bit PCM')
        samples = np.frombuffer(w.readframes(w.getnframes()), dtype=np.int16)
        if w.getnchannels() > 1:
            samples = samples.reshape(-1, w.getnchannels()).mean(axis=1)
        return samples.astype(np.float32) / 32768, w.getframerate()


def dump(data: bytes):
    os.makedirs(DUMP_DIR, exist_ok=True)
    with open(os.path.join(DUMP_DIR, time.strftime('%Y%m%d-%H%M%S.wav')), 'wb') as f:
        f.write(data)
    files = sorted(f for f in os.listdir(DUMP_DIR) if f.endswith('.wav'))
    for name in files[:-DUMP_KEEP]:
        os.remove(os.path.join(DUMP_DIR, name))


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != '/asr':
            return self.reply(404, {'error': 'not found'})
        if recognizer is None:
            return self.reply(503, {'error': 'model is loading'})

        size = int(self.headers.get('Content-Length') or 0)
        if size <= 44 or size > MAX_BYTES:
            return self.reply(400, {'error': 'bad size'})

        data = self.rfile.read(size)
        try:
            samples, rate = read_wav(data)
        except (wave.Error, ValueError, EOFError) as e:
            return self.reply(400, {'error': str(e)})

        started = time.time()
        with lock:
            stream = recognizer.create_stream()
            stream.accept_waveform(rate, samples)
            recognizer.decode_stream(stream)
            text = stream.result.text.strip()
        peak = float(np.abs(samples).max(initial=0))
        rms = float(np.sqrt(np.mean(samples ** 2))) if len(samples) else 0.0
        self.log_message('%.1fs audio at %d Hz in %.2fs, peak %.3f, rms %.4f',
                         len(samples) / rate, rate, time.time() - started, peak, rms)
        if DUMP_DIR:
            self.log_message('text: %s', text)
            dump(data)
        self.reply(200, {'text': text})

    def reply(self, code, body):
        data = json.dumps(body, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json; charset=utf-8')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main():
    global recognizer
    server = ThreadingHTTPServer(('127.0.0.1', PORT), Handler)
    started = time.time()
    recognizer = load()
    print(f'model loaded in {time.time() - started:.1f}s, listening on 127.0.0.1:{PORT}', flush=True)
    server.serve_forever()


if __name__ == '__main__':
    main()
