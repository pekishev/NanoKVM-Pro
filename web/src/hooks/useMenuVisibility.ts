import { useCallback, useEffect, useRef, useState } from 'react';
import { useAtom, useAtomValue } from 'jotai';

import { getMenuDisplayMode } from '@/lib/localstorage.ts';
import { isMacroPlayingAtom } from '@/jotai/mouse.ts';
import { menuDisplayModeAtom, submenuOpenCountAtom } from '@/jotai/settings.ts';

const HIDE_TIMEOUT = 5000;

export interface MenuVisibilityState {
  isMenuExpanded: boolean;
  isMenuHidden: boolean;
  isMenuMoved: boolean;
  handleHovered: (hovered: boolean) => void;
  handleMoved: () => void;
  setIsMenuExpanded: (expanded: boolean) => void;
}

export function useMenuVisibility(): MenuVisibilityState {
  const [menuDisplayMode, setMenuDisplayMode] = useAtom(menuDisplayModeAtom);
  const submenuOpenCount = useAtomValue(submenuOpenCountAtom);
  const isMacroPlaying = useAtomValue(isMacroPlayingAtom);

  const [isMenuExpanded, setIsMenuExpanded] = useState(true);
  const [isMenuMoved, setIsMenuMoved] = useState(false);
  const [isMenuHovered, setIsMenuHovered] = useState(false);
  const [isMenuHidden, setIsMenuHidden] = useState(false);

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isMacroPlayingRef = useRef(isMacroPlaying);
  isMacroPlayingRef.current = isMacroPlaying;

  const startCountdown = useCallback(() => {
    if (
      isMacroPlayingRef.current ||
      menuDisplayMode !== 'auto' ||
      submenuOpenCount > 0 ||
      !isMenuExpanded ||
      isMenuMoved
    ) {
      return;
    }

    if (timerRef.current) {
      clearTimeout(timerRef.current);
    }

    timerRef.current = setTimeout(() => {
      setIsMenuHidden(true);
    }, HIDE_TIMEOUT);
  }, [menuDisplayMode, submenuOpenCount, isMenuExpanded, isMenuMoved]);

  const stopCountdown = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  // Initialize menu settings
  useEffect(() => {
    const displayMode = getMenuDisplayMode();
    setMenuDisplayMode(displayMode);

    if (displayMode === 'off') {
      setIsMenuExpanded(false);
    }

    return () => {
      stopCountdown();
    };
  }, []);

  // Handle display mode changes
  useEffect(() => {
    if (isMacroPlayingRef.current) {
      return;
    }

    setIsMenuHidden(false);

    if (menuDisplayMode === 'auto') {
      startCountdown();
    } else {
      stopCountdown();
    }
  }, [menuDisplayMode, startCountdown, stopCountdown]);

  useEffect(() => {
    if (isMacroPlaying) {
      stopCountdown();
      setIsMenuHidden(true);
      return;
    }

    if (menuDisplayMode === 'always') {
      setIsMenuHidden(false);
    }
  }, [isMacroPlaying, menuDisplayMode, stopCountdown]);

  // Handle hover and submenu state
  useEffect(() => {
    if (submenuOpenCount === 0 && !isMenuHovered) {
      startCountdown();
    } else {
      stopCountdown();
    }
  }, [submenuOpenCount, isMenuHovered, startCountdown, stopCountdown]);

  // Handle hover state
  const handleHovered = useCallback(
    (hovered: boolean) => {
      setIsMenuHovered(hovered);
      if (hovered && !isMacroPlayingRef.current) {
        setIsMenuHidden(false);
      }
    },
    []
  );

  const handleMoved = useCallback(() => {
    if (!isMenuMoved) {
      setIsMenuMoved(true);
    }
  }, [isMenuMoved]);

  return {
    isMenuExpanded,
    isMenuHidden,
    isMenuMoved,
    handleHovered,
    handleMoved,
    setIsMenuExpanded
  };
}
