import { getMacroImage } from '@/api/hid.ts';
import { type ScreenFrame, captureScreenFrame } from '@/lib/screen-capture.ts';

import type { ScreenRect } from './types.ts';

const MAX_EDGE = 96;

// H.264 and MJPEG shift edges by a few levels. A stable fragment stays under this;
// a different screen is usually far above it.
export const MATCH_MAE = 40;

export type MacroImagePayload = ScreenRect & {
  name: string;
  png: string;
};

export type LoadedMacroImage = ScreenRect & {
  width: number;
  height: number;
  data: Uint8ClampedArray;
};

export function cropToPng(frame: ScreenFrame, rect: ScreenRect): string | null {
  const region = regionPixels(frame, rect);
  if (!region) return null;

  const scale = Math.min(1, MAX_EDGE / Math.max(region.w, region.h));
  const dw = Math.max(1, Math.round(region.w * scale));
  const dh = Math.max(1, Math.round(region.h * scale));
  const pixels = scaleCrop(frame, region, dw, dh);
  if (!pixels) return null;

  const canvas = document.createElement('canvas');
  canvas.width = dw;
  canvas.height = dh;
  const ctx = canvas.getContext('2d');
  if (!ctx) return null;
  try {
    ctx.putImageData(toImageData(pixels, dw, dh), 0, 0);
  } catch {
    return null;
  }
  const url = canvas.toDataURL('image/png');
  const comma = url.indexOf(',');
  return comma >= 0 ? url.slice(comma + 1) : null;
}

export async function loadMacroImage(id: string, name: string): Promise<LoadedMacroImage | null> {
  const rsp = await getMacroImage(id, name);
  if (rsp.code !== 0 || !rsp.data?.png) return null;

  const frame = await pngPixels(rsp.data.png);
  if (!frame) return null;

  return {
    x: Number(rsp.data.x),
    y: Number(rsp.data.y),
    w: Number(rsp.data.w),
    h: Number(rsp.data.h),
    width: frame.width,
    height: frame.height,
    data: frame.data
  };
}

// true — match, false — frame seen but different, null — no HDMI frame
export async function regionMatches(reference: LoadedMacroImage): Promise<boolean | null> {
  const frame = await captureScreenFrame();
  if (!frame) return null;

  const region = regionPixels(frame, reference);
  if (!region) return null;

  const live = scaleCrop(frame, region, reference.width, reference.height);
  if (!live) return null;
  return meanAbsDiff(live, reference.data) <= MATCH_MAE;
}

export function meanAbsDiff(a: Uint8ClampedArray, b: Uint8ClampedArray): number {
  const pixels = Math.floor(Math.min(a.length, b.length) / 4);
  if (pixels <= 0 || a.length !== b.length) return 255;

  let sum = 0;
  const bytes = pixels * 4;
  for (let i = 0; i < bytes; i += 4) {
    sum += Math.abs(a[i] - b[i]) + Math.abs(a[i + 1] - b[i + 1]) + Math.abs(a[i + 2] - b[i + 2]);
  }
  return sum / (pixels * 3);
}

function pngPixels(base64: string): Promise<ScreenFrame | null> {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      if (!img.width || !img.height) {
        resolve(null);
        return;
      }
      const canvas = document.createElement('canvas');
      canvas.width = img.width;
      canvas.height = img.height;
      const ctx = canvas.getContext('2d', { willReadFrequently: true });
      if (!ctx) {
        resolve(null);
        return;
      }
      ctx.drawImage(img, 0, 0);
      const image = ctx.getImageData(0, 0, img.width, img.height);
      resolve({ width: img.width, height: img.height, data: image.data });
    };
    img.onerror = () => resolve(null);
    img.src = `data:image/png;base64,${base64}`;
  });
}

type PixelRect = { x: number; y: number; w: number; h: number };

function regionPixels(frame: ScreenFrame, rect: ScreenRect): PixelRect | null {
  if (!frame.width || !frame.height) return null;

  const x = clamp(Math.round((frame.width * rect.x) / 100), 0, frame.width - 1);
  const y = clamp(Math.round((frame.height * rect.y) / 100), 0, frame.height - 1);
  const right = clamp(Math.round((frame.width * (rect.x + rect.w)) / 100), x + 1, frame.width);
  const bottom = clamp(Math.round((frame.height * (rect.y + rect.h)) / 100), y + 1, frame.height);
  return { x, y, w: right - x, h: bottom - y };
}

function scaleCrop(frame: ScreenFrame, region: PixelRect, dw: number, dh: number): Uint8ClampedArray | null {
  if (dw < 1 || dh < 1) return null;

  try {
    const src = document.createElement('canvas');
    src.width = frame.width;
    src.height = frame.height;
    const srcCtx = src.getContext('2d');
    if (!srcCtx) return null;
    srcCtx.putImageData(toImageData(frame.data, frame.width, frame.height), 0, 0);

    const out = document.createElement('canvas');
    out.width = dw;
    out.height = dh;
    const ctx = out.getContext('2d', { willReadFrequently: true });
    if (!ctx) return null;
    ctx.imageSmoothingEnabled = true;
    ctx.drawImage(src, region.x, region.y, region.w, region.h, 0, 0, dw, dh);
    return ctx.getImageData(0, 0, dw, dh).data;
  } catch {
    return null;
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function toImageData(pixels: Uint8ClampedArray, width: number, height: number): ImageData {
  const copy = new Uint8ClampedArray(new ArrayBuffer(pixels.byteLength));
  copy.set(pixels);
  return new ImageData(copy, width, height);
}
