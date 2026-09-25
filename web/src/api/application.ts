import { http } from '@/lib/http.ts';

// get application version
export function getVersion() {
  return http.get('/api/application/version');
}

// update application to the latest official version
export function update() {
  return http.request({
    method: 'post',
    url: '/api/application/update',
    timeout: 15 * 60 * 1000
  });
}

// install an uploaded release archive
export function uploadRelease(formData: FormData, onProgress?: (percent: number) => void) {
  return http.request({
    method: 'post',
    url: '/api/application/upload',
    headers: {
      'Content-Type': 'multipart/form-data'
    },
    data: formData,
    timeout: 15 * 60 * 1000,
    onUploadProgress: (event) => {
      if (!onProgress || !event.total) {
        return;
      }
      onProgress(Math.min(100, Math.round((event.loaded / event.total) * 100)));
    }
  });
}

// enable/disable preview updates
export function setPreviewUpdates(enable: boolean) {
  const data = {
    enable
  };
  return http.post('/api/application/preview', data);
}

// get preview updates state
export function getPreviewUpdates() {
  return http.get('/api/application/preview');
}
