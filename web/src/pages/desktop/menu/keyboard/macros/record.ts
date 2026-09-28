const MODIFIER_TOKENS = new Set(['CTRL', 'SHIFT', 'ALT', 'WIN']);

const CODE_TO_TOKEN: Record<string, string> = {
  ControlLeft: 'CTRL',
  ControlRight: 'CTRL',
  ShiftLeft: 'SHIFT',
  ShiftRight: 'SHIFT',
  AltLeft: 'ALT',
  AltRight: 'ALT',
  MetaLeft: 'WIN',
  MetaRight: 'WIN',
  Enter: 'ENTER',
  NumpadEnter: 'ENTER',
  Escape: 'ESC',
  Backspace: 'BACKSPACE',
  Tab: 'TAB',
  Space: 'SPACE',
  Delete: 'DELETE',
  Insert: 'INSERT',
  Home: 'HOME',
  End: 'END',
  PageUp: 'PAGEUP',
  PageDown: 'PAGEDOWN',
  ArrowUp: 'UP',
  ArrowDown: 'DOWN',
  ArrowLeft: 'LEFT',
  ArrowRight: 'RIGHT',
  CapsLock: 'CAPSLOCK',
  PrintScreen: 'PRINTSCREEN',
  ScrollLock: 'SCROLLLOCK',
  Pause: 'PAUSE',
  ContextMenu: 'MENU',
  NumLock: 'NUMLOCK'
};

export type RecordState = {
  heldCodes: string[];
  gesture: string[];
  sawMain: boolean;
  pendingMain: string | null;
};

export type RecordResult = { line: string } | { unknown: true };

export function createRecordState(): RecordState {
  return { heldCodes: [], gesture: [], sawMain: false, pendingMain: null };
}

export function codeToMacroToken(code: string): string | null {
  if (CODE_TO_TOKEN[code]) {
    return CODE_TO_TOKEN[code];
  }
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(code)) {
    return code;
  }
  if (/^Key[A-Z]$/.test(code)) {
    return code.slice(3);
  }
  if (/^Digit[0-9]$/.test(code)) {
    return code.slice(5);
  }
  return null;
}

export function applyRecordEvent(
  state: RecordState,
  type: 'keydown' | 'keyup',
  code: string,
  repeat = false
): RecordResult | null {
  const token = codeToMacroToken(code);
  if (!token) {
    return type === 'keydown' && code && !repeat ? { unknown: true } : null;
  }

  if (type === 'keydown') {
    if (repeat) {
      return null;
    }

    if (MODIFIER_TOKENS.has(token)) {
      if (!state.heldCodes.includes(code)) {
        state.heldCodes.push(code);
      }
      if (!state.gesture.includes(token)) {
        state.gesture.push(token);
      }
      return null;
    }

    if (state.pendingMain === token) {
      return null;
    }

    state.pendingMain = token;
    state.sawMain = true;
    return { line: [...modifierTokens(state.heldCodes), token].join('+') };
  }

  state.heldCodes = state.heldCodes.filter((held) => held !== code);
  if (state.pendingMain === token) {
    state.pendingMain = null;
  }

  if (state.heldCodes.length > 0 || state.pendingMain) {
    return null;
  }

  const line = !state.sawMain && state.gesture.length ? state.gesture.join('+') : null;
  state.gesture = [];
  state.sawMain = false;
  return line ? { line } : null;
}

function modifierTokens(codes: string[]): string[] {
  const tokens: string[] = [];
  for (const code of codes) {
    const token = codeToMacroToken(code);
    if (token && MODIFIER_TOKENS.has(token) && !tokens.includes(token)) {
      tokens.push(token);
    }
  }
  return tokens;
}
