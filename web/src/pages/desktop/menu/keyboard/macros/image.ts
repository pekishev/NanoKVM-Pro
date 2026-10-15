import { type ScreenFrame } from '@/lib/screen-capture.ts';

import type { ImageRect } from './types.ts';

const MAX_EDGE = 320;

export type MacroImagePayload = ImageRect & {
  name: string;
  png: string;
};

export function cropToPng(frame: ScreenFrame, rect: ImageRect): string | null {
  const region = clipRect(frame, rect);
  if (!region) return null;
  if (region.w > MAX_EDGE || region.h > MAX_EDGE) return null;

  const pixels = copyCrop(frame, region);
  if (!pixels) return null;

  const canvas = document.createElement('canvas');
  canvas.width = region.w;
  canvas.height = region.h;
  const ctx = canvas.getContext('2d');
  if (!ctx) return null;
  try {
    ctx.putImageData(toImageData(pixels, region.w, region.h), 0, 0);
  } catch {
    return null;
  }
  const url = canvas.toDataURL('image/png');
  const comma = url.indexOf(',');
  return comma >= 0 ? url.slice(comma + 1) : null;
}

type PixelRect = { x: number; y: number; w: number; h: number };

function clipRect(frame: ScreenFrame, rect: ImageRect): PixelRect | null {
  if (!frame.width || !frame.height || rect.w < 1 || rect.h < 1) return null;
  const x = clamp(Math.round(rect.x), 0, frame.width - 1);
  const y = clamp(Math.round(rect.y), 0, frame.height - 1);
  const w = clamp(Math.round(rect.w), 1, frame.width - x);
  const h = clamp(Math.round(rect.h), 1, frame.height - y);
  return { x, y, w, h };
}

function copyCrop(frame: ScreenFrame, region: PixelRect): Uint8ClampedArray | null {
  const { x, y, w, h } = region;
  const out = new Uint8ClampedArray(w * h * 4);
  for (let row = 0; row < h; row += 1) {
    const src = ((y + row) * frame.width + x) * 4;
    out.set(frame.data.subarray(src, src + w * 4), row * w * 4);
  }
  return out;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function toImageData(pixels: Uint8ClampedArray, width: number, height: number): ImageData {
  const copy = new Uint8ClampedArray(new ArrayBuffer(pixels.byteLength));
  copy.set(pixels);
  return new ImageData(copy, width, height);
}
