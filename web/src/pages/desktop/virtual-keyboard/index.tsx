import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { AppleOutlined, WindowsOutlined } from '@ant-design/icons';
import clsx from 'clsx';
import { useAtom } from 'jotai';
import { ChevronDownIcon, ChevronUpIcon, GripVerticalIcon, XIcon } from 'lucide-react';
import { KeyboardButtonTheme, KeyboardReact as Keyboard } from 'react-simple-keyboard';
import Draggable, { DraggableData, DraggableEvent } from 'react-draggable';

import 'react-simple-keyboard/build/css/index.css';
import '@/assets/styles/keyboard.css';

import { ConfigProvider, Segmented, Select, theme, Tooltip } from 'antd';
import { useTranslation } from 'react-i18next';
import { useMediaQuery } from 'react-responsive';

import { getKeycode, getModifierBit } from '@/lib/keymap.ts';
import * as storage from '@/lib/localstorage.ts';
import { client, MessageEvent } from '@/lib/websocket.ts';
import { isKeyboardOpenAtom } from '@/jotai/keyboard.ts';

import {
  doubleKeys,
  keyboardArrowsOptions,
  keyboardControlPadOptions,
  keyboardOptions,
  modifierKeys,
  specialKeyMap
} from './virtual-keys.ts';

const MIN_SCALE = 0.55;
const MAX_SCALE = 1.25;

function clampScale(value: number) {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, value));
}

function clampPosition(x: number, y: number, width: number, height: number) {
  return {
    x: Math.min(Math.max(0, x), Math.max(0, window.innerWidth - width)),
    y: Math.min(Math.max(0, y), Math.max(0, window.innerHeight - height))
  };
}

export const VirtualKeyboard = () => {
  const { t } = useTranslation();
  const isBigScreen = useMediaQuery({ minWidth: 850 });
  const baseWidth = isBigScreen ? 820 : 650;

  const [isKeyboardOpen, setIsKeyboardOpen] = useAtom(isKeyboardOpenAtom);

  const [keyboardLayout, setKeyboardLayout] = useState('default');
  const [keyboardSystem, setKeyboardSystem] = useState(() => {
    const system = storage.getKeyboardSystem();
    return system && ['win', 'mac'].includes(system) ? system : 'win';
  });
  const [keyboardLanguage, setKeyboardLanguage] = useState(() => {
    const language = storage.getKeyboardLanguage();
    return language && ['en', 'fr', 'ru'].includes(language) ? language : 'en';
  });
  const [activeModifierKeys, setActiveModifierKeys] = useState<string[]>([]);

  const savedUi = useRef(storage.getVirtualKeyboardUi());
  const nodeRef = useRef<HTMLDivElement | null>(null);
  const keyboardRef = useRef<any>(null);
  const userMovedRef = useRef(!!savedUi.current);

  const [position, setPosition] = useState({
    x: savedUi.current?.x ?? 0,
    y: savedUi.current?.y ?? 0
  });
  const [scale, setScale] = useState(() => clampScale(savedUi.current?.scale ?? 1));
  const [collapsed, setCollapsed] = useState(() => savedUi.current?.collapsed ?? false);
  const [isPlaced, setIsPlaced] = useState(() => !!savedUi.current);

  const stateRef = useRef({ position, scale, collapsed });
  stateRef.current = { position, scale, collapsed };

  const systems = [
    { value: 'win', icon: <WindowsOutlined /> },
    { value: 'mac', icon: <AppleOutlined /> }
  ];

  const languages = [
    { value: 'en', label: 'English' },
    { value: 'fr', label: 'French' },
    { value: 'ru', label: 'Russian' }
  ];

  const persist = useCallback((patch?: Partial<typeof stateRef.current>) => {
    const next = { ...stateRef.current, ...patch };
    storage.setVirtualKeyboardUi({
      x: next.position.x,
      y: next.position.y,
      scale: next.scale,
      collapsed: next.collapsed
    });
  }, []);

  const syncPosition = useCallback(() => {
    if (!nodeRef.current) return;

    const { width, height } = nodeRef.current.getBoundingClientRect();

    if (!userMovedRef.current) {
      const next = {
        x: Math.max(0, (window.innerWidth - width) / 2),
        y: Math.max(0, window.innerHeight - height - 8)
      };
      setPosition((prev) => (prev.x === next.x && prev.y === next.y ? prev : next));
      setIsPlaced(true);
      return;
    }

    setPosition((prev) => {
      const next = clampPosition(prev.x, prev.y, width, height);
      return prev.x === next.x && prev.y === next.y ? prev : next;
    });
  }, []);

  useEffect(() => {
    const layoutMap = new Map([
      ['en', 'default'],
      ['ru', 'rus'],
      ['fr', 'azerty']
    ]);

    if (keyboardLanguage === 'en' && keyboardSystem === 'mac') {
      setKeyboardLayout('mac');
      return;
    }

    if (layoutMap.has(keyboardLanguage)) {
      setKeyboardLayout(layoutMap.get(keyboardLanguage)!);
      return;
    }

    setKeyboardLayout('default');
  }, [keyboardSystem, keyboardLanguage]);

  useLayoutEffect(() => {
    if (!isKeyboardOpen || !nodeRef.current) return;
    syncPosition();

    const observer = new ResizeObserver(() => syncPosition());
    observer.observe(nodeRef.current);
    window.addEventListener('resize', syncPosition);
    return () => {
      observer.disconnect();
      window.removeEventListener('resize', syncPosition);
    };
  }, [isKeyboardOpen, collapsed, scale, isBigScreen, syncPosition]);

  const wasOpenRef = useRef(false);
  useEffect(() => {
    if (wasOpenRef.current && !isKeyboardOpen) {
      persist();
    }
    wasOpenRef.current = isKeyboardOpen;
  }, [isKeyboardOpen, persist]);

  // Press key
  function onKeyPress(key: string) {
    if (modifierKeys.includes(key)) {
      if (activeModifierKeys.includes(key)) {
        sendModifierKeyDown();
        sendModifierKeyUp();
      } else {
        setActiveModifierKeys([...activeModifierKeys, key]);
      }
      return;
    }

    sendKeydown(key);
  }

  // Release key
  function onKeyReleased(key: string) {
    if (modifierKeys.includes(key)) {
      return;
    }

    sendKeyup();
  }

  // Send all keys
  function sendKeydown(key: string) {
    const code = getKeyboardCode(key);
    if (!code) {
      console.log('unknown code: ', key);
      return;
    }

    const modifier = sendModifierKeyDown();

    send(modifier, code);
  }

  function getKeyboardCode(key: string) {
    // AZERTY: swap A↔Q and Z↔W on French physical positions
    if (keyboardLanguage === 'fr' && key.endsWith('_azerty')) {
      const base = key.replace('_azerty', '');
      if (base === 'KeyA') return getKeycode('KeyQ');
      if (base === 'KeyQ') return getKeycode('KeyA');
      if (base === 'KeyZ') return getKeycode('KeyW');
      if (base === 'KeyW') return getKeycode('KeyZ');
      // all other labels use their own code
      return getKeycode(base);
    }

    // Russian: Key*_rus use same physical positions as QWERTY → strip _rus and use base code
    if (keyboardLanguage === 'ru' && key.endsWith('_rus')) {
      const base = key.replace('_rus', '');
      return getKeycode(base);
    }

    const specialKey = specialKeyMap.get(key);
    if (specialKey) {
      return getKeycode(specialKey);
    }

    return getKeycode(key);
  }

  // Release all keys
  function sendKeyup() {
    sendModifierKeyUp();
    send(0, 0);
  }

  // Send modifier keys
  function sendModifierKeyDown() {
    let modifier = 0;

    activeModifierKeys.forEach((modifierKey) => {
      const key = specialKeyMap.get(modifierKey)!;

      modifier |= getModifierBit(key)!;
      const code = getKeycode(key)!;

      send(modifier, code);
    });

    return modifier;
  }

  // Release modifier keys
  function sendModifierKeyUp() {
    if (activeModifierKeys.length === 0) return;

    activeModifierKeys.forEach(() => {
      send(0, 0);
    });

    setActiveModifierKeys([]);
  }

  function send(modifier: number, code: number) {
    const data = new Uint8Array([MessageEvent.Keyboard, modifier, 0, code, 0, 0, 0, 0, 0]);
    client.send(data);
  }

  function selectSystem(system: string) {
    setKeyboardSystem(system);
    storage.setKeyboardSystem(system);
  }

  function selectLanguage(language: string) {
    setKeyboardLanguage(language);
    storage.setKeyboardLanguage(language);
  }

  function getButtonTheme(): KeyboardButtonTheme[] {
    const theme = [{ class: 'hg-double', buttons: doubleKeys.join(' ') }];

    if (activeModifierKeys.length > 0) {
      const buttons = activeModifierKeys.join(' ');
      theme.push({ class: 'hg-highlight', buttons });
    }

    return theme;
  }

  function onDrag(_e: DraggableEvent, data: DraggableData) {
    userMovedRef.current = true;
    setPosition({ x: data.x, y: data.y });
  }

  function onDragStop(_e: DraggableEvent, data: DraggableData) {
    userMovedRef.current = true;
    const next = { x: data.x, y: data.y };
    setPosition(next);
    persist({ position: next });
  }

  function toggleCollapsed() {
    const next = !collapsed;
    setCollapsed(next);
    persist({ collapsed: next });
  }

  function closeKeyboard() {
    persist();
    setIsKeyboardOpen(false);
  }

  function onResizePointerDown(event: React.PointerEvent<HTMLDivElement>) {
    event.preventDefault();
    event.stopPropagation();

    const startX = event.clientX;
    const startScale = scale;
    const startWidth = baseWidth * startScale;
    let currentScale = startScale;

    const onMove = (ev: PointerEvent) => {
      currentScale = clampScale((startWidth + (ev.clientX - startX)) / baseWidth);
      setScale(Number(currentScale.toFixed(3)));
    };

    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
      persist({ scale: currentScale });
    };

    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  }

  if (!isKeyboardOpen) return null;

  return (
    <Draggable
      nodeRef={nodeRef}
      handle=".vk-drag-handle"
      cancel=".vk-no-drag"
      bounds="html"
      position={position}
      onDrag={onDrag}
      onStop={onDragStop}
    >
      <div
        ref={nodeRef}
        className={clsx(
          'fixed left-0 top-0 z-[999] overflow-hidden rounded bg-white shadow-xl outline-none',
          isPlaced ? 'opacity-100' : 'opacity-0'
        )}
        role="dialog"
        aria-label={t('keyboard.virtual')}
        style={{ width: baseWidth * scale }}
      >
        <div
          className="vk-drag-handle flex cursor-move items-center justify-between whitespace-nowrap px-3 py-2"
          role="toolbar"
          tabIndex={0}
          aria-label={t('keyboard.virtual')}
          onDoubleClick={toggleCollapsed}
        >
          <ConfigProvider
            theme={{
              algorithm: theme.defaultAlgorithm
            }}
          >
            <div className="flex items-center space-x-3">
              <div
                className="flex flex-shrink-0 items-center text-neutral-400"
                role="button"
                aria-label="Move"
              >
                <GripVerticalIcon size={16} />
              </div>
              <div className="vk-no-drag flex items-center space-x-5">
                <Select
                  size="small"
                  style={{ minWidth: 90 }}
                  value={keyboardLanguage}
                  options={languages}
                  onChange={selectLanguage}
                />

                {keyboardLanguage === 'en' && (
                  <Segmented
                    size="small"
                    options={systems}
                    value={keyboardSystem}
                    onChange={selectSystem}
                  />
                )}
              </div>
            </div>
          </ConfigProvider>

          <div className="vk-no-drag flex items-center justify-end space-x-1">
            <Tooltip
              title={collapsed ? t('keyboard.expand') : t('keyboard.collapse')}
              getPopupContainer={() => document.body}
            >
              <div
                className="flex h-[20px] w-[20px] cursor-pointer items-center justify-center rounded text-neutral-600 hover:bg-neutral-300 hover:text-white"
                role="button"
                aria-label={collapsed ? t('keyboard.expand') : t('keyboard.collapse')}
                onClick={toggleCollapsed}
              >
                {collapsed ? <ChevronUpIcon size={18} /> : <ChevronDownIcon size={18} />}
              </div>
            </Tooltip>
            <div
              className="flex h-[20px] w-[20px] cursor-pointer items-center justify-center rounded text-neutral-600 hover:bg-neutral-300 hover:text-white"
              role="button"
              aria-label="Close"
              onClick={closeKeyboard}
            >
              <XIcon size={18} />
            </div>
          </div>
        </div>

        {!collapsed && (
          <>
            <div className="h-px flex-shrink-0 border-b bg-neutral-300" />

            <div className="relative">
              <div className="keyboardContainer" style={{ zoom: scale, width: baseWidth }}>
                {/* main keyboard */}
                <Keyboard
                  buttonTheme={getButtonTheme()}
                  keyboardRef={(r) => (keyboardRef.current = r)}
                  onKeyPress={onKeyPress}
                  onKeyReleased={onKeyReleased}
                  layoutName={keyboardLayout}
                  {...keyboardOptions}
                />

                {/* control keyboard */}
                {isBigScreen && (
                  <div className="controlArrows">
                    <Keyboard
                      onKeyPress={onKeyPress}
                      onKeyReleased={onKeyReleased}
                      {...keyboardControlPadOptions}
                    />

                    <Keyboard
                      onKeyPress={onKeyPress}
                      onKeyReleased={onKeyReleased}
                      {...keyboardArrowsOptions}
                    />
                  </div>
                )}
              </div>

              <div
                className="vk-resize-handle vk-no-drag"
                aria-hidden="true"
                onPointerDown={onResizePointerDown}
              />
            </div>
          </>
        )}
      </div>
    </Draggable>
  );
};
