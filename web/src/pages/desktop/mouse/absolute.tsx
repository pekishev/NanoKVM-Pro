import { useEffect, useRef } from 'react';
import { getDefaultStore, useAtomValue } from 'jotai';
import { useMediaQuery } from 'react-responsive';

import { MouseReportAbsolute } from '@/lib/mouse.ts';
import { client, MessageEvent } from '@/lib/websocket.ts';
import {
  isMacroPlayingAtom,
  mouseModeAtom,
  scrollDirectionAtom,
  scrollIntervalAtom
} from '@/jotai/mouse.ts';

import { MouseAbsoluteEvent } from './types.ts';

enum MouseButton {
  Left = 0,
  Middle = 1,
  Right = 2,
  Back = 3,
  Forward = 4
}

export const Absolute = () => {
  const isBigScreen = useMediaQuery({ minWidth: 650 });

  const mouseMode = useAtomValue(mouseModeAtom);
  const isSimple = mouseMode === 'absolute-simple';
  const scrollDirection = useAtomValue(scrollDirectionAtom);
  const scrollInterval = useAtomValue(scrollIntervalAtom);

  const mouseRef = useRef(new MouseReportAbsolute());
  const lastPosRef = useRef({ x: 0.5, y: 0.5 });
  const lastScrollTimeRef = useRef(0);

  // For touch events
  const touchStartTimeRef = useRef(0);
  const lastTouchYRef = useRef(0);
  const longPressTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isLongPressRef = useRef(false);
  const hasMoveRef = useRef(false);
  const isDraggingRef = useRef(false);
  const pressedButtonRef = useRef<MouseButton | null>(null);
  const touchStartPosRef = useRef({ x: 0, y: 0 });
  const ignoreMouseUntilRef = useRef(0);

  const TAP_THRESHOLD = 8;
  const DRAG_THRESHOLD = 10;
  const VELOCITY_THRESHOLD = 0.3;

  useEffect(() => {
    const screen = document.getElementById('screen') as HTMLVideoElement;
    if (!screen) return;

    screen.addEventListener('mousedown', handleMouseDown);
    screen.addEventListener('mouseup', handleMouseUp);
    screen.addEventListener('mousemove', handleMouseMove);
    screen.addEventListener('wheel', handleWheel);
    screen.addEventListener('click', disableEvent);
    screen.addEventListener('contextmenu', disableEvent);

    const gestureSurface = screen.parentElement ?? screen;
    const previousOverflow = document.documentElement.style.overflow;
    const view = { scale: 1, x: 0, y: 0 };
    const twoFinger = {
      mode: 'pending' as 'pending' | 'pinch' | 'scroll',
      startDist: 0,
      startMidX: 0,
      startMidY: 0,
      lastMidY: 0,
      startScale: 1,
      startX: 0,
      startY: 0,
      layoutLeft: 0,
      layoutTop: 0
    };
    const pan = { active: false, startX: 0, startY: 0, originX: 0, originY: 0 };
    let touchOnScreen = false;

    if (isSimple) {
      gestureSurface.addEventListener('touchstart', handleSimpleTouchStart, { passive: false });
      gestureSurface.addEventListener('touchmove', handleSimpleTouchMove, { passive: false });
      gestureSurface.addEventListener('touchend', handleSimpleTouchEnd, { passive: false });
      gestureSurface.addEventListener('touchcancel', handleSimpleTouchCancel, { passive: false });
      gestureSurface.style.touchAction = 'none';
      document.documentElement.style.overflow = 'hidden';
    } else if (isBigScreen) {
      screen.addEventListener('touchstart', handleTouchStart);
      screen.addEventListener('touchmove', handleTouchMove);
      screen.addEventListener('touchend', handleTouchEnd);
      screen.addEventListener('touchcancel', handleTouchCancel);
    }

    function firesTouchEvents(event: Event) {
      const capabilities = (
        event as Event & { sourceCapabilities?: { firesTouchEvents?: boolean } }
      ).sourceCapabilities;
      return Boolean(capabilities?.firesTouchEvents);
    }

    function shouldIgnoreMouse(e: MouseEvent) {
      if (!isSimple) {
        return false;
      }
      if (Date.now() < ignoreMouseUntilRef.current) {
        return true;
      }
      return firesTouchEvents(e);
    }

    function holdOffSyntheticMouse() {
      ignoreMouseUntilRef.current = Date.now() + 700;
    }

    // Mouse down event
    function handleMouseDown(e: MouseEvent) {
      if (shouldIgnoreMouse(e)) {
        return;
      }
      disableEvent(e);
      handleMouseEvent({ type: 'mousedown', button: e.button });
    }

    // Mouse up event
    function handleMouseUp(e: MouseEvent) {
      if (shouldIgnoreMouse(e)) {
        return;
      }
      disableEvent(e);
      handleMouseEvent({ type: 'mouseup', button: e.button });
    }

    // Mouse move event
    function handleMouseMove(e: MouseEvent) {
      if (shouldIgnoreMouse(e)) {
        return;
      }
      disableEvent(e);
      const { x, y } = getCoordinate(e);
      handleMouseEvent({ type: 'move', x, y });
    }

    // Mouse wheel event
    function handleWheel(e: WheelEvent) {
      if (shouldIgnoreMouse(e)) {
        return;
      }
      disableEvent(e);

      if (Math.floor(e.deltaY) === 0) {
        return;
      }

      const currentTime = Date.now();
      if (currentTime - lastScrollTimeRef.current < scrollInterval) {
        return;
      }

      const deltaY = (e.deltaY > 0 ? 1 : -1) * scrollDirection;
      handleMouseEvent({ type: 'wheel', deltaY });
      lastScrollTimeRef.current = currentTime;
    }

    const MIN_SCALE = 1;
    const MAX_SCALE = 5;
    const PINCH_ARM = 18;
    const SCROLL_ARM = 28;

    function fingerDistance(touches: TouchList) {
      const dx = touches[0].clientX - touches[1].clientX;
      const dy = touches[0].clientY - touches[1].clientY;
      return Math.hypot(dx, dy);
    }

    function midpoint(touches: TouchList) {
      return {
        x: (touches[0].clientX + touches[1].clientX) / 2,
        y: (touches[0].clientY + touches[1].clientY) / 2
      };
    }

    function clearLongPress() {
      if (longPressTimerRef.current) {
        clearTimeout(longPressTimerRef.current);
        longPressTimerRef.current = null;
      }
    }

    function applyView() {
      if (view.scale <= MIN_SCALE) {
        view.scale = MIN_SCALE;
        view.x = 0;
        view.y = 0;
        gestureSurface.style.transform = '';
        gestureSurface.style.transformOrigin = '';
        return;
      }

      gestureSurface.style.transformOrigin = '0 0';
      gestureSurface.style.transform = `translate(${view.x}px, ${view.y}px) scale(${view.scale})`;
    }

    function isOverScreen(clientX: number, clientY: number) {
      const el = document.elementFromPoint(clientX, clientY);
      return !!el && (el === screen || screen.contains(el));
    }

    function beginTwoFinger(touches: TouchList) {
      const rect = gestureSurface.getBoundingClientRect();
      const mid = midpoint(touches);
      twoFinger.layoutLeft = rect.left - view.x;
      twoFinger.layoutTop = rect.top - view.y;
      twoFinger.startDist = fingerDistance(touches);
      twoFinger.startMidX = mid.x;
      twoFinger.startMidY = mid.y;
      twoFinger.lastMidY = mid.y;
      twoFinger.startScale = view.scale;
      twoFinger.startX = view.x;
      twoFinger.startY = view.y;
      twoFinger.mode = 'pending';
    }

    function updatePinch(touches: TouchList) {
      const dist = fingerDistance(touches);
      const mid = midpoint(touches);
      if (twoFinger.startDist <= 0) {
        return;
      }

      let scale = twoFinger.startScale * (dist / twoFinger.startDist);
      scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, scale));

      if (scale <= MIN_SCALE) {
        view.scale = MIN_SCALE;
        view.x = 0;
        view.y = 0;
        applyView();
        return;
      }

      const contentX =
        (twoFinger.startMidX - twoFinger.layoutLeft - twoFinger.startX) / twoFinger.startScale;
      const contentY =
        (twoFinger.startMidY - twoFinger.layoutTop - twoFinger.startY) / twoFinger.startScale;
      view.scale = scale;
      view.x = mid.x - twoFinger.layoutLeft - contentX * scale;
      view.y = mid.y - twoFinger.layoutTop - contentY * scale;
      applyView();
    }

    function sendSimpleWheel(deltaClientY: number) {
      if (deltaClientY === 0) {
        return;
      }

      const currentTime = Date.now();
      if (currentTime - lastScrollTimeRef.current < scrollInterval) {
        return;
      }

      const deltaY = (deltaClientY > 0 ? 1 : -1) * scrollDirection;
      handleMouseEvent({ type: 'wheel', deltaY });
      lastScrollTimeRef.current = currentTime;
    }

    // Absolute Simple keeps browser zoom out of the way: pinch and pan are our own
    // transform, so they still work when fullscreen locks the visual viewport.
    function handleSimpleTouchStart(e: TouchEvent) {
      if (e.touches.length === 0) {
        return;
      }

      holdOffSyntheticMouse();
      clearLongPress();

      if (pressedButtonRef.current !== null) {
        handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current });
        pressedButtonRef.current = null;
      }

      const touch = e.touches[0];
      touchStartTimeRef.current = Date.now();
      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pan.active = false;
      touchStartPosRef.current = { x: touch.clientX, y: touch.clientY };

      if (e.touches.length > 1) {
        e.preventDefault();
        hasMoveRef.current = true;
        touchOnScreen = false;
        beginTwoFinger(e.touches);
        return;
      }

      twoFinger.mode = 'pending';
      twoFinger.startDist = 0;
      touchOnScreen = isOverScreen(touch.clientX, touch.clientY);
      if (!touchOnScreen) {
        return;
      }

      longPressTimerRef.current = setTimeout(() => {
        isLongPressRef.current = true;
        pressedButtonRef.current = MouseButton.Right;
        if (navigator.vibrate) {
          navigator.vibrate(50);
        }

        const { x, y } = getCoordinate({
          clientX: touchStartPosRef.current.x,
          clientY: touchStartPosRef.current.y
        });
        handleMouseEvent({ type: 'move', x, y });
        handleMouseEvent({ type: 'mousedown', button: MouseButton.Right });
      }, 800);
    }

    function handleSimpleTouchMove(e: TouchEvent) {
      if (e.touches.length === 0) {
        return;
      }

      holdOffSyntheticMouse();

      if (e.touches.length > 1) {
        e.preventDefault();
        clearLongPress();
        hasMoveRef.current = true;
        pan.active = false;

        if (twoFinger.startDist <= 0) {
          beginTwoFinger(e.touches);
          return;
        }

        const distance = fingerDistance(e.touches);
        const mid = midpoint(e.touches);
        const distDelta = Math.abs(distance - twoFinger.startDist);
        const scrollDelta = Math.abs(mid.y - twoFinger.startMidY);

        if (twoFinger.mode === 'pending') {
          if (distDelta > PINCH_ARM && distDelta >= scrollDelta) {
            twoFinger.mode = 'pinch';
          } else if (scrollDelta > SCROLL_ARM && scrollDelta > distDelta * 1.5) {
            twoFinger.mode = 'scroll';
          } else {
            return;
          }
        }

        if (twoFinger.mode === 'pinch') {
          updatePinch(e.touches);
          return;
        }

        sendSimpleWheel(mid.y - twoFinger.lastMidY);
        twoFinger.lastMidY = mid.y;
        return;
      }

      const touch = e.touches[0];
      const deltaX = touch.clientX - touchStartPosRef.current.x;
      const deltaY = touch.clientY - touchStartPosRef.current.y;
      if (Math.hypot(deltaX, deltaY) <= TAP_THRESHOLD) {
        return;
      }

      hasMoveRef.current = true;
      clearLongPress();

      if (view.scale <= MIN_SCALE) {
        return;
      }

      e.preventDefault();
      if (!pan.active) {
        pan.active = true;
        pan.startX = touch.clientX;
        pan.startY = touch.clientY;
        pan.originX = view.x;
        pan.originY = view.y;
      }

      view.x = pan.originX + (touch.clientX - pan.startX);
      view.y = pan.originY + (touch.clientY - pan.startY);
      applyView();
    }

    function handleSimpleTouchEnd(e: TouchEvent) {
      holdOffSyntheticMouse();
      clearLongPress();

      if (e.touches.length > 0) {
        hasMoveRef.current = true;
        pan.active = false;
        if (pressedButtonRef.current !== null) {
          handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current });
          pressedButtonRef.current = null;
        }
        if (e.touches.length === 1) {
          twoFinger.startDist = 0;
          twoFinger.mode = 'pending';
        }
        return;
      }

      const touch = e.changedTouches[0];
      if (
        touch &&
        touchOnScreen &&
        !hasMoveRef.current &&
        !isLongPressRef.current &&
        twoFinger.mode !== 'pinch' &&
        twoFinger.mode !== 'scroll'
      ) {
        const { x, y } = getCoordinate(touch);
        handleMouseEvent({ type: 'move', x, y });
        handleMouseEvent({ type: 'mousedown', button: MouseButton.Left });
        setTimeout(() => {
          handleMouseEvent({ type: 'mouseup', button: MouseButton.Left });
        }, 50);
      } else if (pressedButtonRef.current !== null) {
        handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current });
      }

      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pressedButtonRef.current = null;
      pan.active = false;
      touchOnScreen = false;
      twoFinger.mode = 'pending';
      twoFinger.startDist = 0;
    }

    function handleSimpleTouchCancel() {
      holdOffSyntheticMouse();
      clearLongPress();

      if (pressedButtonRef.current !== null) {
        handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current });
      }

      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pressedButtonRef.current = null;
      pan.active = false;
      touchOnScreen = false;
      twoFinger.mode = 'pending';
      twoFinger.startDist = 0;
    }

    // Mouse touch start event
    function handleTouchStart(e: TouchEvent) {
      disableEvent(e);

      if (e.touches.length === 0) {
        return;
      }

      const touch = e.touches[0];

      // Reset states
      touchStartTimeRef.current = Date.now();
      lastTouchYRef.current = touch.clientY;
      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pressedButtonRef.current = null;
      touchStartPosRef.current = { x: touch.clientX, y: touch.clientY };

      if (longPressTimerRef.current) {
        clearTimeout(longPressTimerRef.current);
      }

      const { x, y } = getCoordinate(touch);
      handleMouseEvent({ type: 'move', x, y });

      if (e.touches.length > 1) {
        return;
      }

      // Start long press
      longPressTimerRef.current = setTimeout(() => {
        isLongPressRef.current = true;
        pressedButtonRef.current = MouseButton.Right;
        if (navigator.vibrate) {
          navigator.vibrate(50);
        }

        handleMouseEvent({ type: 'mousedown', button: MouseButton.Right });
      }, 800);
    }

    // Mouse touch move event
    function handleTouchMove(e: TouchEvent) {
      disableEvent(e);

      if (e.touches.length === 0) {
        return;
      }
      const touch = e.touches[0];

      // Handle two-finger scroll first
      if (e.touches.length > 1) {
        const currentTime = Date.now();
        if (currentTime - lastScrollTimeRef.current < scrollInterval) {
          return;
        }

        const deltaY = (touch.clientY - lastTouchYRef.current > 0 ? 1 : -1) * scrollDirection;
        handleMouseEvent({ type: 'wheel', deltaY });

        lastTouchYRef.current = touch.clientY;
        lastScrollTimeRef.current = currentTime;
        return;
      }

      const deltaX = Math.abs(touch.clientX - touchStartPosRef.current.x);
      const deltaY = Math.abs(touch.clientY - touchStartPosRef.current.y);
      const distance = Math.sqrt(deltaX * deltaX + deltaY * deltaY);

      const timeDelta = Date.now() - touchStartTimeRef.current;
      const velocity = timeDelta > 0 ? distance / timeDelta : 0;

      const shouldStartDrag =
        distance > DRAG_THRESHOLD || (distance > TAP_THRESHOLD && velocity > VELOCITY_THRESHOLD);

      if (shouldStartDrag && !isDraggingRef.current && !isLongPressRef.current) {
        if (!hasMoveRef.current) {
          hasMoveRef.current = true;
        }

        if (longPressTimerRef.current) {
          clearTimeout(longPressTimerRef.current);
          longPressTimerRef.current = null;
        }

        if (pressedButtonRef.current === null) {
          isDraggingRef.current = true;
          pressedButtonRef.current = MouseButton.Left;
          handleMouseEvent({ type: 'mousedown', button: MouseButton.Left });
        }
      }

      if (distance > TAP_THRESHOLD && !hasMoveRef.current) {
        hasMoveRef.current = true;
      }

      if (isDraggingRef.current || isLongPressRef.current) {
        const { x, y } = getCoordinate(touch);
        handleMouseEvent({ type: 'move', x, y });
      }
    }

    // Mouse touch end event
    function handleTouchEnd(e: TouchEvent) {
      disableEvent(e);

      if (longPressTimerRef.current) {
        clearTimeout(longPressTimerRef.current);
        longPressTimerRef.current = null;
      }

      if (!hasMoveRef.current && !isLongPressRef.current) {
        handleMouseEvent({ type: 'mousedown', button: MouseButton.Left });
        setTimeout(() => {
          handleMouseEvent({ type: 'mouseup', button: MouseButton.Left });
        }, 50);
      } else if (pressedButtonRef.current !== null) {
        handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current! });
      }

      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pressedButtonRef.current = null;
    }

    // Mouse touch cancel event
    function handleTouchCancel(e: any) {
      disableEvent(e);

      if (longPressTimerRef.current) {
        clearTimeout(longPressTimerRef.current);
        longPressTimerRef.current = null;
      }

      if (pressedButtonRef.current) {
        handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current! });
      }

      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pressedButtonRef.current = null;
    }

    // get mouse coordinate
    function getCoordinate(event: any) {
      const { x, y } = getCorrectedCoords(event.clientX, event.clientY);

      const finalX = Math.max(0, Math.min(1, x));
      const finalY = Math.max(0, Math.min(1, y));

      const hexX = Math.floor(0x7fff * finalX) + 0x0001;
      const hexY = Math.floor(0x7fff * finalY) + 0x0001;

      return { x: hexX, y: hexY };
    }

    function getCorrectedCoords(clientX: number, clientY: number) {
      const rect = screen.getBoundingClientRect();

      if (!screen.videoWidth || !screen.videoHeight) {
        const x = (clientX - rect.left) / rect.width;
        const y = (clientY - rect.top) / rect.height;
        return { x, y };
      }

      const videoRatio = screen.videoWidth / screen.videoHeight;
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

      return { x, y };
    }

    return () => {
      screen.removeEventListener('mousemove', handleMouseMove);
      screen.removeEventListener('mousedown', handleMouseDown);
      screen.removeEventListener('mouseup', handleMouseUp);
      screen.removeEventListener('wheel', handleWheel);
      screen.removeEventListener('click', disableEvent);
      screen.removeEventListener('contextmenu', disableEvent);
      screen.removeEventListener('touchstart', handleTouchStart);
      screen.removeEventListener('touchmove', handleTouchMove);
      screen.removeEventListener('touchend', handleTouchEnd);
      screen.removeEventListener('touchcancel', handleTouchCancel);
      gestureSurface.removeEventListener('touchstart', handleSimpleTouchStart);
      gestureSurface.removeEventListener('touchmove', handleSimpleTouchMove);
      gestureSurface.removeEventListener('touchend', handleSimpleTouchEnd);
      gestureSurface.removeEventListener('touchcancel', handleSimpleTouchCancel);
      gestureSurface.style.transform = '';
      gestureSurface.style.transformOrigin = '';
      gestureSurface.style.touchAction = '';
      document.documentElement.style.overflow = previousOverflow;

      if (longPressTimerRef.current) {
        clearTimeout(longPressTimerRef.current);
      }
    };
  }, [isBigScreen, isSimple, scrollDirection, scrollInterval]);

  // Mouse event handler
  function handleMouseEvent(event: MouseAbsoluteEvent) {
    if (getDefaultStore().get(isMacroPlayingAtom)) {
      return;
    }

    let report: Uint8Array;
    const mouse = mouseRef.current;

    switch (event.type) {
      case 'mousedown':
        mouse.buttonDown(event.button);
        report = mouse.buildButtonReport(lastPosRef.current.x, lastPosRef.current.y);
        break;
      case 'mouseup':
        mouse.buttonUp(event.button);
        report = mouse.buildButtonReport(lastPosRef.current.x, lastPosRef.current.y);
        break;
      case 'wheel':
        report = mouse.buildReport(lastPosRef.current.x, lastPosRef.current.y, event.deltaY);
        break;
      case 'move':
        report = mouse.buildReport(event.x, event.y);
        lastPosRef.current = { x: event.x, y: event.y };
        break;
      default:
        report = mouse.buildReport(lastPosRef.current.x, lastPosRef.current.y);
        break;
    }

    sendReport(report);
  }

  function sendReport(report: Uint8Array) {
    const data = new Uint8Array([MessageEvent.Mouse, ...report]);
    client.send(data);
  }

  // disable default events
  function disableEvent(event: any) {
    event.preventDefault();
    event.stopPropagation();
  }

  return <></>;
};
