import { getMacroImage } from '@/api/hid.ts';
import { type ScreenFrame, captureScreenFrame } from '@/lib/screen-capture.ts';

import type { ScreenRect } from './types.ts';

const MAX_EDGE = 96;

// Matching runs in reference pixels. Mean RGB difference alone cannot tell a small
// light-on-dark label from an empty background, so structure is compared on luma
// normalized by mean/std (ZNCC) and additionally per grid cell, so one changed glyph
// is not averaged away. Thresholds are calibrated against H.264/MJPEG artifacts and
// 2x rescaling: a true match stays within them, a changed glyph lands far outside.
// Luma hides hue (green and yellow icons of one shape correlate perfectly), so colour
// is checked separately as per-cell chroma; 4:2:0 subsampling keeps a match under ~40.
const SEARCH_PX = 2;
const GRID = 3;
const MIN_ZNCC = 0.75;
const MAX_CELL_RESIDUAL = 1.7;
const MAX_CELL_CHROMA = 50;
const MIN_CONTRAST = 0.5;
const MAX_CONTRAST = 2;
const MAX_MAE = 28;
const FLAT_STD = 4;
const FLAT_MAE = 12;

export type MacroImagePayload = ScreenRect & {
  name: string;
  png: string;
};

export type LoadedMacroImage = ScreenRect & {
  width: number;
  height: number;
  data: Uint8ClampedArray;
  luma: Float32Array;
  mean: number;
  std: number;
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

  const pixels = frame.width * frame.height;
  const luma = new Float32Array(pixels);
  let sum = 0;
  for (let i = 0; i < pixels; i++) {
    const k = i * 4;
    luma[i] = toLuma(frame.data[k], frame.data[k + 1], frame.data[k + 2]);
    sum += luma[i];
  }
  const mean = sum / pixels;
  let variance = 0;
  for (let i = 0; i < pixels; i++) {
    variance += (luma[i] - mean) ** 2;
  }

  return {
    x: Number(rsp.data.x),
    y: Number(rsp.data.y),
    w: Number(rsp.data.w),
    h: Number(rsp.data.h),
    width: frame.width,
    height: frame.height,
    data: frame.data,
    luma,
    mean,
    std: Math.sqrt(variance / pixels)
  };
}

// true — match, false — frame seen but different, null — no HDMI frame
export async function regionMatches(reference: LoadedMacroImage): Promise<boolean | null> {
  const frame = await captureScreenFrame();
  if (!frame) return null;

  const region = regionPixels(frame, reference);
  if (!region) return null;

  const kx = reference.width / region.w;
  const ky = reference.height / region.h;
  const left = Math.max(0, region.x - Math.ceil(SEARCH_PX / kx));
  const top = Math.max(0, region.y - Math.ceil(SEARCH_PX / ky));
  const right = Math.min(frame.width, region.x + region.w + Math.ceil(SEARCH_PX / kx));
  const bottom = Math.min(frame.height, region.y + region.h + Math.ceil(SEARCH_PX / ky));

  const winW = Math.max(reference.width, Math.round((right - left) * kx));
  const winH = Math.max(reference.height, Math.round((bottom - top) * ky));
  const win = scaleCrop(frame, { x: left, y: top, w: right - left, h: bottom - top }, winW, winH);
  if (!win) return null;

  const ox = Math.round((region.x - left) * kx);
  const oy = Math.round((region.y - top) * ky);
  const maxDx = Math.min(winW - reference.width, ox + SEARCH_PX);
  const maxDy = Math.min(winH - reference.height, oy + SEARCH_PX);
  for (let dy = Math.max(0, oy - SEARCH_PX); dy <= maxDy; dy++) {
    for (let dx = Math.max(0, ox - SEARCH_PX); dx <= maxDx; dx++) {
      if (matchesAt(win, winW, dx, dy, reference)) return true;
    }
  }
  return false;
}

function matchesAt(
  win: Uint8ClampedArray,
  winW: number,
  dx: number,
  dy: number,
  ref: LoadedMacroImage
): boolean {
  const { width: w, height: h } = ref;
  const pixels = w * h;
  const live = new Float32Array(pixels);
  const cellChroma = new Float64Array(GRID * GRID);
  const cellCount = new Uint32Array(GRID * GRID);
  let sum = 0;
  let colorDiff = 0;
  for (let y = 0; y < h; y++) {
    const row = Math.floor((y * GRID) / h) * GRID;
    for (let x = 0; x < w; x++) {
      const i = y * w + x;
      const j = ((y + dy) * winW + x + dx) * 4;
      const k = i * 4;
      const r = win[j];
      const g = win[j + 1];
      const b = win[j + 2];
      const l = toLuma(r, g, b);
      live[i] = l;
      sum += l;
      colorDiff +=
        Math.abs(r - ref.data[k]) + Math.abs(g - ref.data[k + 1]) + Math.abs(b - ref.data[k + 2]);

      const rl = ref.luma[i];
      const cell = row + Math.floor((x * GRID) / w);
      cellChroma[cell] += Math.max(
        Math.abs(r - l - (ref.data[k] - rl)),
        Math.abs(g - l - (ref.data[k + 1] - rl)),
        Math.abs(b - l - (ref.data[k + 2] - rl))
      );
      cellCount[cell]++;
    }
  }

  const mae = colorDiff / (pixels * 3);
  if (mae > MAX_MAE) return false;
  for (let c = 0; c < cellChroma.length; c++) {
    if (cellCount[c] && cellChroma[c] / cellCount[c] > MAX_CELL_CHROMA) return false;
  }

  const mean = sum / pixels;
  let variance = 0;
  for (let i = 0; i < pixels; i++) {
    variance += (live[i] - mean) ** 2;
  }
  const std = Math.sqrt(variance / pixels);

  if (ref.std < FLAT_STD) {
    return mae <= FLAT_MAE && std < ref.std + FLAT_STD;
  }
  if (std < ref.std * MIN_CONTRAST || std > ref.std * MAX_CONTRAST) return false;

  const cellSum = new Float64Array(GRID * GRID);
  let cov = 0;
  for (let y = 0; y < h; y++) {
    const row = Math.floor((y * GRID) / h) * GRID;
    for (let x = 0; x < w; x++) {
      const i = y * w + x;
      const a = (live[i] - mean) / std;
      const b = (ref.luma[i] - ref.mean) / ref.std;
      cov += a * b;
      const cell = row + Math.floor((x * GRID) / w);
      cellSum[cell] += (a - b) ** 2;
    }
  }
  if (cov / pixels < MIN_ZNCC) return false;

  for (let c = 0; c < cellSum.length; c++) {
    if (cellCount[c] && cellSum[c] / cellCount[c] > MAX_CELL_RESIDUAL) return false;
  }
  return true;
}

function toLuma(r: number, g: number, b: number): number {
  return r * 0.299 + g * 0.587 + b * 0.114;
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
