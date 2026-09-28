import { type PointerEvent as ReactPointerEvent, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';

import { clientToScreenPercent } from '@/lib/coords.ts';

import type { ScreenRect } from './types.ts';

type RegionPickerProps = {
  open: boolean;
  onPick: (rect: ScreenRect) => void;
  onCancel: () => void;
};

type Box = { left: number; top: number; width: number; height: number };

export const RegionPicker = ({ open, onPick, onCancel }: RegionPickerProps) => {
  const { t } = useTranslation();
  const startRef = useRef<{ x: number; y: number } | null>(null);
  const [box, setBox] = useState<Box | null>(null);

  useEffect(() => {
    if (!open) {
      startRef.current = null;
      setBox(null);
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        event.stopPropagation();
        onCancel();
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [open, onCancel]);

  if (!open) return null;

  function onPointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    startRef.current = { x: event.clientX, y: event.clientY };
    setBox({ left: event.clientX, top: event.clientY, width: 0, height: 0 });
  }

  function onPointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const start = startRef.current;
    if (!start) return;
    setBox(boxBetween(start.x, start.y, event.clientX, event.clientY));
  }

  function onPointerUp(event: ReactPointerEvent<HTMLDivElement>) {
    const start = startRef.current;
    startRef.current = null;
    setBox(null);
    if (!start) return;
    if (Math.abs(event.clientX - start.x) < 8 || Math.abs(event.clientY - start.y) < 8) return;

    const from = clientToScreenPercent(start.x, start.y);
    const to = clientToScreenPercent(event.clientX, event.clientY);
    if (!from || !to) {
      onCancel();
      return;
    }

    const x = Math.min(from.x, to.x);
    const y = Math.min(from.y, to.y);
    const w = Math.round(Math.abs(from.x - to.x) * 100) / 100;
    const h = Math.round(Math.abs(from.y - to.y) * 100) / 100;
    if (w < 0.3 || h < 0.3) return;

    onPick({ x, y, w, h });
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[3000] cursor-crosshair touch-none bg-black/20"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onContextMenu={(event) => {
        event.preventDefault();
        onCancel();
      }}
    >
      <div className="pointer-events-none absolute left-1/2 top-4 -translate-x-1/2 rounded bg-neutral-900/90 px-4 py-2 text-sm text-neutral-100 shadow">
        {t('keyboard.macro.imagePickHint')}
      </div>
      {box && box.width > 0 && box.height > 0 && (
        <div
          className="pointer-events-none absolute border-2 border-blue-400 bg-blue-400/20"
          style={{ left: box.left, top: box.top, width: box.width, height: box.height }}
        />
      )}
    </div>,
    document.body
  );
};

function boxBetween(x0: number, y0: number, x1: number, y1: number): Box {
  return {
    left: Math.min(x0, x1),
    top: Math.min(y0, y1),
    width: Math.abs(x1 - x0),
    height: Math.abs(y1 - y0)
  };
}
