import { getDefaultStore } from 'jotai';

import { KeyboardReport } from '@/lib/keyboard.ts';
import { MouseReportAbsolute } from '@/lib/mouse.ts';
import { percentToHid } from '@/lib/coords.ts';
import { client, MessageEvent } from '@/lib/websocket.ts';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { isMacroPlayingAtom } from '@/jotai/mouse.ts';

import { charToCodes } from './chars.ts';
import { loadMacroImage, regionMatches } from './image.ts';
import { parseScript } from './script.ts';
import type { Macro, MouseButtonName, ScriptCommand } from './types.ts';

const IMAGE_POLL_MS = 200;
// A single frame can be mid-redraw; require the fragment on consecutive polls.
const IMAGE_CONFIRM_FRAMES = 2;
const IMAGE_CONFIRM_MS = 100;

// Keyboard report pacing. A plain key is down, hold, then release.
// STRING adds KEY_GAP_MS between characters. Mouse timing is unchanged.
const KEY_DOWN_MS = 50;
const KEY_HOLD_MS = 80;
const KEY_GAP_MS = 80;
const COMMAND_GAP_MS = 100;

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

async function moveTo(mouse: MouseReportAbsolute, x: number, y: number) {
  sendMouse(mouse.buildReport(percentToHid(x), percentToHid(y), 0));
  await sleep(30);
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
    await sleep(KEY_DOWN_MS);
  }

  await sleep(KEY_HOLD_MS);
  sendKeyboard(keyboard.reset());
}

async function typeText(text: string) {
  for (const char of text) {
    const codes = charToCodes(char);
    if (!codes) {
      continue;
    }
    await pressCodes(codes);
    await sleep(KEY_GAP_MS);
  }
}

export async function playMacro(macro: Macro, macros: Macro[] = []): Promise<string | null> {
  const store = getDefaultStore();
  const previousKeyboard = store.get(isKeyboardEnableAtom);

  store.set(isKeyboardEnableAtom, false);
  store.set(isMacroPlayingAtom, true);

  try {
    return await playCommands(macro, macros, new MouseReportAbsolute(), 0);
  } finally {
    store.set(isMacroPlayingAtom, false);
    store.set(isKeyboardEnableAtom, previousKeyboard);
  }
}

async function playCommands(
  macro: Macro,
  macros: Macro[],
  mouse: MouseReportAbsolute,
  depth: number
): Promise<string | null> {
  const parsed = parseScript(macro.script);
  if (!parsed.ok) {
    return `${macro.name}: line ${parsed.line}: ${parsed.error}`;
  }

  for (const command of parsed.commands) {
    const error = await playCommand(command, macro, macros, mouse, depth);
    if (error) {
      return error;
    }

    if (command.type !== 'delay') {
      await sleep(COMMAND_GAP_MS);
    }
  }

  return null;
}

async function playCommand(
  command: ScriptCommand,
  macro: Macro,
  macros: Macro[],
  mouse: MouseReportAbsolute,
  depth: number
): Promise<string | null> {
  switch (command.type) {
    case 'move':
      await moveTo(mouse, command.x, command.y);
      return null;
    case 'click':
      await clickAt(mouse, command.x, command.y, command.button, command.times);
      return null;
    case 'key':
      await pressCodes(command.codes);
      return null;
    case 'string':
      await typeText(command.text);
      return null;
    case 'delay':
      await sleep(command.ms);
      return null;
    case 'run':
      return playNestedMacro(command.name, macro, macros, mouse, depth);
    case 'waitimage':
      return waitForImage(macro, command.name, command.ms);
  }
}

async function waitForImage(macro: Macro, name: string, timeoutMs: number): Promise<string | null> {
  if (!macro.id) {
    return `${macro.name}: image not found: ${name}`;
  }

  const reference = await loadMacroImage(macro.id, name);
  if (!reference) {
    return `${macro.name}: image not found: ${name}`;
  }

  const deadline = Date.now() + timeoutMs;
  let sawFrame = false;
  let streak = 0;

  for (;;) {
    const matched = await regionMatches(reference);
    if (matched !== null) {
      sawFrame = true;
    }
    if (matched === true) {
      streak++;
      if (streak >= IMAGE_CONFIRM_FRAMES) {
        return null;
      }
      await sleep(IMAGE_CONFIRM_MS);
      continue;
    }
    streak = 0;

    const now = Date.now();
    if (now >= deadline) {
      if (!sawFrame) {
        return `${macro.name}: HDMI frame unavailable`;
      }
      return `${macro.name}: WAITIMAGE ${name} timed out`;
    }
    await sleep(Math.min(IMAGE_POLL_MS, deadline - now));
  }
}

async function playNestedMacro(
  name: string,
  parent: Macro,
  macros: Macro[],
  mouse: MouseReportAbsolute,
  depth: number
): Promise<string | null> {
  if (depth >= 1) {
    return `${parent.name}: nested RUN is limited to 1 level`;
  }

  const child = findMacro(macros, name);
  if (!child) {
    return `${parent.name}: macro not found: ${name}`;
  }

  return playCommands(child, macros, mouse, depth + 1);
}

function findMacro(macros: Macro[], name: string): Macro | undefined {
  const needle = name.trim().toLowerCase();
  return macros.find((macro) => macro.name.trim().toLowerCase() === needle);
}
