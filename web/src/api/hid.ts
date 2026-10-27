import { http } from '@/lib/http.ts';

// paste
export function paste(content: string) {
  return http.post('/api/hid/paste', { content });
}

// recognize speech on the device
export function recognizeSpeech(wav: Blob) {
  return http.request({
    method: 'post',
    url: '/api/hid/speech',
    data: wav,
    headers: { 'Content-Type': 'audio/wav' },
    timeout: 90 * 1000
  });
}

// reset hid
export function reset() {
  return http.post('/api/hid/reset');
}

// get hid mode
export function getHidMode() {
  return http.get('/api/hid/mode');
}

// set hid mode
export function setHidMode(mode: string) {
  const data = {
    mode
  };
  return http.post('/api/hid/mode', data);
}

export function getMacros() {
  return http.get('/api/hid/macros');
}

export function addMacro(name: string, script: string) {
  return http.post('/api/hid/macro', { name, script });
}

export function updateMacro(id: string, name: string, script: string) {
  return http.post('/api/hid/macro/update', { id, name, script });
}

export function deleteMacro(id: string) {
  return http.delete('/api/hid/macro', { id });
}

export function saveMacroImage(image: {
  id: string;
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
  frameW: number;
  frameH: number;
  png: string;
}) {
  return http.post('/api/hid/macro/image', image);
}

export function captureMacroImage(rect: {
  x: number;
  y: number;
  w: number;
  h: number;
  frameW: number;
  frameH: number;
}) {
  return http.post('/api/hid/macro/image/capture', rect);
}

export function getMacroImage(id: string, name: string) {
  return http.get('/api/hid/macro/image', { id, name });
}

export function runMacro(payload: { id?: string; name?: string }) {
  return http.post('/api/hid/macro/run', payload, { timeout: 6 * 60 * 1000 });
}

export function stopMacro() {
  return http.post('/api/hid/macro/stop');
}

export function waitImage(payload: {
  id?: string;
  macro?: string;
  name?: string;
  ms?: number;
  x?: number;
  y?: number;
  w?: number;
  h?: number;
  frameW?: number;
  frameH?: number;
  png?: string;
}) {
  return http.post('/api/hid/waitimage', payload, { timeout: 6 * 60 * 1000 });
}
