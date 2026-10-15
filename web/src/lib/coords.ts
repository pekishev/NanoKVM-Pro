import { getDefaultStore } from 'jotai';

import { videoParametersAtom } from '@/jotai/screen.ts';
import { getScreenElement, inverseRotatePoint, isQuarterTurn } from '@/lib/video-transform.ts';
import type { VideoRotation } from '@/types';

export function clientToScreenPercent(
  clientX: number,
  clientY: number
): { x: number; y: number } | null {
  const point = clientToScreenPoint(clientX, clientY);
  if (!point) {
    return null;
  }
  return {
    x: roundPercent(point.nx * 100),
    y: roundPercent(point.ny * 100)
  };
}

export function clientToScreenPixel(
  clientX: number,
  clientY: number
): { x: number; y: number; width: number; height: number } | null {
  const point = clientToScreenPoint(clientX, clientY);
  if (!point || !point.width || !point.height) {
    return null;
  }
  return {
    x: clampInt(Math.round(point.nx * point.width), 0, point.width - 1),
    y: clampInt(Math.round(point.ny * point.height), 0, point.height - 1),
    width: point.width,
    height: point.height
  };
}

function clientToScreenPoint(clientX: number, clientY: number) {
  const screen = getScreenElement();
  if (!screen) {
    return null;
  }

  const rotation = getDefaultStore().get(videoParametersAtom).rotation;
  const mediaSize = getMediaSize(screen);
  const { x, y } = getCorrectedCoords(screen, clientX, clientY, rotation);
  return {
    nx: x,
    ny: y,
    width: mediaSize?.width ?? 0,
    height: mediaSize?.height ?? 0
  };
}

export function percentToHid(percent: number): number {
  const n = Math.max(0, Math.min(100, percent)) / 100;
  return Math.floor(0x7fff * n) + 0x0001;
}

function getCorrectedCoords(
  screen: Element,
  clientX: number,
  clientY: number,
  rotation: VideoRotation
) {
  const rect = screen.getBoundingClientRect();
  const mediaSize = getMediaSize(screen);

  if (!mediaSize || !rect.width || !rect.height) {
    const x = rect.width ? (clientX - rect.left) / rect.width : 0;
    const y = rect.height ? (clientY - rect.top) / rect.height : 0;
    const rotated = inverseRotatePoint(x, y, rotation);
    return { x: clamp01(rotated.x), y: clamp01(rotated.y) };
  }

  const rotatedMediaSize = isQuarterTurn(rotation)
    ? { width: mediaSize.height, height: mediaSize.width }
    : mediaSize;
  const videoRatio = rotatedMediaSize.width / rotatedMediaSize.height;
  const elementRatio = rect.width / rect.height;

  let renderedWidth = rect.width;
  let renderedHeight = rect.height;
  let offsetX = 0;
  let offsetY = 0;

  if (videoRatio > elementRatio) {
    renderedHeight = rect.width / videoRatio;
    offsetY = (rect.height - renderedHeight) / 2;
  } else {
    renderedWidth = rect.height * videoRatio;
    offsetX = (rect.width - renderedWidth) / 2;
  }

  const x = (clientX - rect.left - offsetX) / renderedWidth;
  const y = (clientY - rect.top - offsetY) / renderedHeight;
  const rotated = inverseRotatePoint(x, y, rotation);
  return { x: clamp01(rotated.x), y: clamp01(rotated.y) };
}

function getMediaSize(screen: Element): { width: number; height: number } | null {
  if (screen instanceof HTMLVideoElement && screen.videoWidth > 0 && screen.videoHeight > 0) {
    return { width: screen.videoWidth, height: screen.videoHeight };
  }
  if (screen instanceof HTMLImageElement && screen.naturalWidth > 0 && screen.naturalHeight > 0) {
    return { width: screen.naturalWidth, height: screen.naturalHeight };
  }
  if (screen instanceof HTMLCanvasElement && screen.width > 0 && screen.height > 0) {
    return { width: screen.width, height: screen.height };
  }
  return null;
}

function clamp01(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.max(0, Math.min(1, value));
}

function roundPercent(value: number): number {
  return Math.round(clamp01(value / 100) * 10000) / 100;
}

function clampInt(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}
