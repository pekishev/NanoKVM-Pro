import { useEffect, useRef } from 'react';
import { getDefaultStore, useAtomValue } from 'jotai';
import { useMediaQuery } from 'react-responsive';

import { MouseReportAbsolute } from '@/lib/mouse.ts';
import { getScreenElement, inverseRotatePoint, isQuarterTurn } from '@/lib/video-transform.ts';
import {
  isMacroPlayingAtom,
  mouseModeAtom,
  scrollDirectionAtom,
  scrollIntervalAtom
} from '@/jotai/mouse.ts';
import { videoModeAtom, videoParametersAtom } from '@/jotai/screen.ts';

import {
  registerMouseReleaseHandler,
  sendMouseRelease,
  sendMouseReport
} from './lifecycle.ts';
import { MouseAbsoluteEvent } from './types.ts';

enum MouseButton {
  Left = 0,
  Middle = 1,
  Right = 2,
  Back = 3,
  Forward = 4
}

interface ReleaseMouseOptions {
  force?: boolean;
  cancelTouchSequence?: boolean;
}

export const Absolute = () => {
  const isBigScreen = useMediaQuery({ minWidth: 650 });

  const mouseMode = useAtomValue(mouseModeAtom);
  const isSimple = mouseMode === 'absolute-simple';
  const scrollDirection = useAtomValue(scrollDirectionAtom);
  const scrollInterval = useAtomValue(scrollIntervalAtom);
  const videoMode = useAtomValue(videoModeAtom);
  const videoParameters = useAtomValue(videoParametersAtom);

  const mouseRef = useRef(new MouseReportAbsolute());
  // Absolute reports store HID coordinates, not normalized CSS coordinates.
  const lastPosRef = useRef({ x: 0x4000, y: 0x4000 });
  const lastScrollTimeRef = useRef(0);

  // For touch events
  const touchStartTimeRef = useRef(0);
  const lastTouchYRef = useRef(0);
  const longPressTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const tapReleaseTimersRef = useRef(new Set<ReturnType<typeof setTimeout>>());
  const isLongPressRef = useRef(false);
  const hasMoveRef = useRef(false);
  const isDraggingRef = useRef(false);
  const isMultiTouchRef = useRef(false);
  // These refs prevent lifecycle-canceled touches from becoming a late tap/drag.
  const activeTouchCountRef = useRef(0);
  const isTouchSequenceCanceledRef = useRef(false);
  const pressedButtonRef = useRef<MouseButton | null>(null);
  const touchStartPosRef = useRef({ x: 0, y: 0 });
  const ignoreMouseUntilRef = useRef(0);

  const TAP_THRESHOLD = 8;
  const DRAG_THRESHOLD = 10;
  const VELOCITY_THRESHOLD = 0.3;

  useEffect(() => {
    const screenElement = getScreenElement();
    if (!screenElement) return;
    const screen = screenElement;

    screen.addEventListener('mousedown', handleMouseDown);
    screen.addEventListener('mouseup', handleMouseUp);
    screen.addEventListener('mousemove', handleMouseMove);
    screen.addEventListener('wheel', handleWheel);
    screen.addEventListener('click', disableEvent);
    screen.addEventListener('contextmenu', disableEvent);
    window.addEventListener('blur', handleWindowBlur);
    window.addEventListener('mouseup', handleWindowMouseUp);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    const unregisterMouseReleaseHandler = registerMouseReleaseHandler(releaseMouse);

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
      lastPosRef.current = getCoordinate(e);
      handleMouseEvent({ type: 'mousedown', button: e.button });
    }

    // Mouse up event
    function handleMouseUp(e: MouseEvent) {
      if (shouldIgnoreMouse(e)) {
        return;
      }
      disableEvent(e);
      lastPosRef.current = getCoordinate(e);
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

    function handleWindowBlur() {
      releaseMouse();
    }

    function handleWindowMouseUp(e: MouseEvent) {
      // Screen mouseup stops propagation; this catches releases outside the video element.
      if (mouseRef.current.hasPressedButtons) {
        handleMouseEvent({ type: 'mouseup', button: e.button });
      }
    }

    function handleVisibilityChange() {
      if (document.hidden) {
        releaseMouse();
      }
    }

    function clearTapReleaseTimers() {
      // Multiple fast taps can have overlapping delayed releases.
      for (const timer of tapReleaseTimersRef.current) {
        clearTimeout(timer);
      }
      tapReleaseTimersRef.current.clear();
    }

    function releaseMouse(options: ReleaseMouseOptions = {}) {
      const { force = false, cancelTouchSequence = true } = options;
      const mouse = mouseRef.current;
      const hadPressedButtons = mouse.hasPressedButtons;
      const report = mouse.reset(lastPosRef.current.x, lastPosRef.current.y);

      // Keep the cancellation gate until every still-active browser touch has ended.
      if (cancelTouchSequence && activeTouchCountRef.current > 0) {
        isTouchSequenceCanceledRef.current = true;
      }

      clearLongPress();
      clearTapReleaseTimers();

      touchStartTimeRef.current = 0;
      lastTouchYRef.current = 0;
      lastScrollTimeRef.current = 0;
      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      isMultiTouchRef.current = false;
      pressedButtonRef.current = null;
      touchStartPosRef.current = { x: 0, y: 0 };

      // Reset local state first; queue the host-visible neutral report if transport is down.
      if (hadPressedButtons || force) {
        sendMouseRelease('absolute', report);
      }
    }

    function handleMouseEvent(event: MouseAbsoluteEvent): boolean {
      if (getDefaultStore().get(isMacroPlayingAtom)) {
        return false;
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

      const sent = sendMouseReport('absolute', report);
      if (!sent && (event.type === 'mouseup' || mouse.hasPressedButtons)) {
        releaseMouse({ force: true });
      }

      return sent;
    }

    // Mouse touch start event
    function handleTouchStart(e: TouchEvent) {
      disableEvent(e);

      if (e.touches.length === 0) {
        return;
      }

      const startsNewSequence = e.touches.length === e.changedTouches.length;
      if (startsNewSequence) {
        // Browsers may omit the old final touchend; settle all stale state before a new gesture.
        releaseMouse({ cancelTouchSequence: false });
        isTouchSequenceCanceledRef.current = false;
      }
      activeTouchCountRef.current = e.touches.length;
      if (isTouchSequenceCanceledRef.current) {
        return;
      }

      const touch = e.touches[0];

      if (e.touches.length > 1) {
        // Entering multi-touch ends any single-finger drag/long-press before scrolling.
        releaseMouse({ cancelTouchSequence: false });
        isMultiTouchRef.current = true;
        lastTouchYRef.current = touch.clientY;
        return;
      }

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
      if (!handleMouseEvent({ type: 'move', x, y })) {
        // Do not create a long-press timer for a gesture whose initial position was not sent.
        isTouchSequenceCanceledRef.current = true;
        return;
      }

      // Start long press
      longPressTimerRef.current = setTimeout(() => {
        longPressTimerRef.current = null;
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

      activeTouchCountRef.current = e.touches.length;
      if (isTouchSequenceCanceledRef.current) {
        return;
      }

      if (e.touches.length === 0) {
        return;
      }
      const touch = e.touches[0];

      // Once a gesture becomes multi-touch, do not reinterpret the remaining
      // finger as a tap or drag after another finger is lifted.
      if (isMultiTouchRef.current) {
        if (e.touches.length < 2) {
          return;
        }

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

      activeTouchCountRef.current = e.touches.length;

      if (longPressTimerRef.current) {
        clearTimeout(longPressTimerRef.current);
        longPressTimerRef.current = null;
      }

      if (isTouchSequenceCanceledRef.current) {
        // A lifecycle release owns this sequence until its final touch disappears.
        if (e.touches.length === 0) {
          isTouchSequenceCanceledRef.current = false;
        }
        return;
      }

      if (isMultiTouchRef.current) {
        if (e.touches.length === 0) {
          isMultiTouchRef.current = false;
          lastTouchYRef.current = 0;
          lastScrollTimeRef.current = 0;
        }
        return;
      }

      if (!hasMoveRef.current && !isLongPressRef.current) {
        const sent = handleMouseEvent({ type: 'mousedown', button: MouseButton.Left });
        if (!sent) {
          return;
        }
        const timer = setTimeout(() => {
          handleMouseEvent({ type: 'mouseup', button: MouseButton.Left });
          tapReleaseTimersRef.current.delete(timer);
        }, 50);
        tapReleaseTimersRef.current.add(timer);
      } else if (pressedButtonRef.current !== null) {
        handleMouseEvent({ type: 'mouseup', button: pressedButtonRef.current! });
      }

      isLongPressRef.current = false;
      hasMoveRef.current = false;
      isDraggingRef.current = false;
      pressedButtonRef.current = null;
    }

    // Mouse touch cancel event
    function handleTouchCancel(e: TouchEvent) {
      disableEvent(e);
      activeTouchCountRef.current = e.touches.length;
      releaseMouse();
      if (e.touches.length === 0) {
        isTouchSequenceCanceledRef.current = false;
      }
    }

    // get mouse coordinate
    function getCoordinate(event: any) {
      const { x, y } = getCorrectedCoords(event.clientX, event.clientY);

      const finalX = Math.max(0, Math.min(1, x));
      const finalY = Math.max(0, Math.min(1, y));

      // Map rendered video space onto the GS2 15-bit absolute coordinate range.
      const hexX = Math.round(0x7fff * finalX);
      const hexY = Math.round(0x7fff * finalY);

      return { x: hexX, y: hexY };
    }

    function getCorrectedCoords(clientX: number, clientY: number) {
      const rect = screen.getBoundingClientRect();
      const mediaSize = getMediaSize(screen);

      if (!mediaSize) {
        const x = (clientX - rect.left) / rect.width;
        const y = (clientY - rect.top) / rect.height;
        return inverseRotatePoint(x, y, videoParameters.rotation);
      }

      const rotatedMediaSize = isQuarterTurn(videoParameters.rotation)
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

      return inverseRotatePoint(x, y, videoParameters.rotation);
    }

    return () => {
      releaseMouse();
      unregisterMouseReleaseHandler();

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
      window.removeEventListener('blur', handleWindowBlur);
      window.removeEventListener('mouseup', handleWindowMouseUp);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
    };
  }, [isBigScreen, isSimple, scrollDirection, scrollInterval, videoMode, videoParameters.rotation]);

  // disable default events
  function disableEvent(event: any) {
    event.preventDefault();
    event.stopPropagation();
  }

  return <></>;
};

function getMediaSize(screen: Element) {
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
