import { http } from '@/lib/http.ts';

// paste
export function paste(content: string) {
  return http.post('/api/hid/paste', { content });
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
