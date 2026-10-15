import { useState } from 'react';
import { notification } from 'antd';
import type { TFunction } from 'i18next';
import { getDefaultStore, useAtomValue, useSetAtom } from 'jotai';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/hid.ts';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { isMacroPlayingAtom } from '@/jotai/mouse.ts';

import { countScriptCommands } from './script.ts';
import type { Macro } from './types.ts';

type MacroItemProps = {
  macro: Macro;
};

export const MacroItem = ({ macro }: MacroItemProps) => {
  const { t } = useTranslation();
  const isPlaying = useAtomValue(isMacroPlayingAtom);
  const setPlaying = useSetAtom(isMacroPlayingAtom);
  const setKeyboardEnable = useSetAtom(isKeyboardEnableAtom);
  const [isLoading, setIsLoading] = useState(false);

  async function handleClick() {
    if (isLoading || isPlaying || !macro.id || !macro.script.trim()) return;

    const store = getDefaultStore();
    const previousKeyboard = store.get(isKeyboardEnableAtom);

    setIsLoading(true);
    setKeyboardEnable(false);
    setPlaying(true);

    try {
      const rsp = await api.runMacro({ id: macro.id });
      if (rsp.code === 0 || rsp.code === -4) {
        return;
      }
      notification.error({
        message: describePlayError(rsp.msg, t),
        placement: 'topRight',
        duration: 6
      });
    } catch (err) {
      console.log(err);
    } finally {
      setPlaying(false);
      setKeyboardEnable(previousKeyboard);
      setIsLoading(false);
    }
  }

  return (
    <div
      className="flex h-[32px] w-full cursor-pointer items-center justify-between space-x-3 rounded px-3 hover:bg-neutral-700/30"
      onClick={handleClick}
    >
      <span className="truncate">{macro.name}</span>
      <span className="text-xs text-neutral-500">
        {isLoading
          ? t('keyboard.macro.playing')
          : t('keyboard.macro.stepsCount', { count: countScriptCommands(macro.script) })}
      </span>
    </div>
  );
};

function describePlayError(error: string, t: TFunction): string {
  const timeout = /^(.+): WAITIMAGE (\S+) timed out$/.exec(error);
  if (timeout) {
    return t('keyboard.macro.imageTimeout', { macro: timeout[1], name: timeout[2] });
  }
  const missing = /^(.+): image not found: (\S+)$/.exec(error);
  if (missing) {
    return t('keyboard.macro.imageMissing', { macro: missing[1], name: missing[2] });
  }
  const frame = /^(.+): HDMI frame unavailable$/.exec(error);
  if (frame) {
    return t('keyboard.macro.imageNoFrame', { macro: frame[1] });
  }
  return error;
}
