import { getDefaultStore } from 'jotai';

import { KeyboardReport } from '@/lib/keyboard.ts';
import { MouseReportAbsolute } from '@/lib/mouse.ts';
import { percentToHid } from '@/lib/coords.ts';
import { client, MessageEvent } from '@/lib/websocket.ts';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { isMacroPlayingAtom } from '@/jotai/mouse.ts';

import { charToCodes } from './chars.ts';
import { parseScript } from './script.ts';
import type { Macro, MouseButtonName } from './types.ts';

const BUTTON_INDEX: Record<MouseButtonName, number> = {
  left: 0,
  middle: 1,
  right: 2
};

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function sendKeyboard(report: Uint8Array) {
  client.send(new Uint8Array([MessageEvent.Keyboard, ...report]));
}

function sendMouse(report: Uint8Array) {
  client.send(new Uint8Array([MessageEvent.Mouse, ...report]));
}

async function clickAt(
  mouse: MouseReportAbsolute,
  x: number,
  y: number,
  button: MouseButtonName,
  times: number
) {
  const hidX = percentToHid(x);
  const hidY = percentToHid(y);
  const buttonIndex = BUTTON_INDEX[button] ?? 0;

  sendMouse(mouse.buildReport(hidX, hidY, 0));
  await sleep(30);

  for (let i = 0; i < times; i++) {
    mouse.buttonDown(buttonIndex);
    sendMouse(mouse.buildButtonReport(hidX, hidY));
    await sleep(50);

    mouse.buttonUp(buttonIndex);
    sendMouse(mouse.buildButtonReport(hidX, hidY));

    if (i < times - 1) {
      await sleep(80);
    }
  }
}

async function pressCodes(codes: string[]) {
  const keyboard = new KeyboardReport();

  for (const code of codes) {
    sendKeyboard(keyboard.keyDown(code));
    await sleep(20);
  }

  await sleep(50);
  sendKeyboard(keyboard.reset());
}

async function typeText(text: string) {
  for (const char of text) {
    const codes = charToCodes(char);
    if (!codes) {
      continue;
    }
    await pressCodes(codes);
    await sleep(50);
  }
}

export async function playMacro(macro: Macro): Promise<string | null> {
  const parsed = parseScript(macro.script);
  if (!parsed.ok) {
    return `Line ${parsed.line}: ${parsed.error}`;
  }

  const store = getDefaultStore();
  const previousKeyboard = store.get(isKeyboardEnableAtom);

  store.set(isKeyboardEnableAtom, false);
  store.set(isMacroPlayingAtom, true);

  try {
    const mouse = new MouseReportAbsolute();

    for (const command of parsed.commands) {
      switch (command.type) {
        case 'click':
          await clickAt(mouse, command.x, command.y, command.button, command.times);
          break;
        case 'key':
          await pressCodes(command.codes);
          break;
        case 'string':
          await typeText(command.text);
          break;
        case 'delay':
          await sleep(command.ms);
          break;
      }

      if (command.type !== 'delay') {
        await sleep(30);
      }
    }
  } finally {
    store.set(isMacroPlayingAtom, false);
    store.set(isKeyboardEnableAtom, previousKeyboard);
  }

  return null;
}
