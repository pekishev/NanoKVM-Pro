export type ScreenFrame = {
  width: number;
  height: number;
  data: Uint8ClampedArray;
};

type ScreenCapturer = () => Promise<ScreenFrame | null>;

let screenCapturer: ScreenCapturer | null = null;

export function setScreenCapturer(next: ScreenCapturer | null) {
  screenCapturer = next;
}

export function captureFromWorker(worker: Worker): Promise<ScreenFrame | null> {
  return new Promise((resolve) => {
    const timer = window.setTimeout(() => {
      worker.removeEventListener('message', onMessage);
      resolve(null);
    }, 1000);

    const onMessage = (event: MessageEvent) => {
      if (event.data?.type !== 'snapshot') return;
      window.clearTimeout(timer);
      worker.removeEventListener('message', onMessage);
      if (!event.data.ok || !event.data.buffer) {
        resolve(null);
        return;
      }
      resolve({
        width: event.data.width,
        height: event.data.height,
        data: new Uint8ClampedArray(event.data.buffer)
      });
    };

    worker.addEventListener('message', onMessage);
    worker.postMessage({ type: 'snapshot' });
  });
}

export async function captureScreenFrame(): Promise<ScreenFrame | null> {
  const media = findScreenMedia();
  if (!media) return null;

  if (media instanceof HTMLCanvasElement) {
    if (!screenCapturer) return null;
    return screenCapturer();
  }

  return captureMediaElement(media);
}

function findScreenMedia(): HTMLVideoElement | HTMLImageElement | HTMLCanvasElement | null {
  const screen = document.getElementById('screen');
  if (!screen) return null;
  if (
    screen instanceof HTMLVideoElement ||
    screen instanceof HTMLImageElement ||
    screen instanceof HTMLCanvasElement
  ) {
    return screen;
  }
  return screen.querySelector('video, img, canvas');
}

function captureMediaElement(media: HTMLVideoElement | HTMLImageElement): ScreenFrame | null {
  const width = media instanceof HTMLVideoElement ? media.videoWidth : media.naturalWidth;
  const height = media instanceof HTMLVideoElement ? media.videoHeight : media.naturalHeight;
  if (!width || !height) return null;

  try {
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });
    if (!ctx) return null;
    ctx.drawImage(media, 0, 0, width, height);
    const image = ctx.getImageData(0, 0, width, height);
    return { width, height, data: image.data };
  } catch {
    return null;
  }
}
