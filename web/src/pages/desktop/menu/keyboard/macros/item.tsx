import { useState } from 'react';
import { notification } from 'antd';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

import { playMacro } from './player.ts';
import { countScriptCommands } from './script.ts';
import type { Macro } from './types.ts';

type MacroItemProps = {
  macro: Macro;
  macros: Macro[];
};

export const MacroItem = ({ macro, macros }: MacroItemProps) => {
  const { t } = useTranslation();
  const [isLoading, setIsLoading] = useState(false);

  async function handleClick() {
    if (isLoading || !macro.script.trim()) return;
    setIsLoading(true);

    try {
      const error = await playMacro(macro, macros);
      if (error) {
        console.log(error);
        notification.error({
          message: describePlayError(error, t),
          placement: 'topRight',
          duration: 6
        });
      }
    } catch (err) {
      console.log(err);
    } finally {
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
