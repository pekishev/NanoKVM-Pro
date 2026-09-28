import { ChangeEvent, useRef, useState } from 'react';
import { Button, Input, Modal, Select, Tooltip, type InputRef } from 'antd';
import clsx from 'clsx';
import { useSetAtom } from 'jotai';
import { ClipboardIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { paste } from '@/api/hid';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { KeyboardReport } from '@/lib/keyboard.ts';
import { client, MessageEvent } from '@/lib/websocket.ts';

const { TextArea } = Input;

type Layout = 'en' | 'ru';

const languages = [
  { value: 'en', labelKey: 'keyboard.dropdownEnglish' },
  { value: 'ru', labelKey: 'keyboard.dropdownRussian' }
];

// Physical US keys that produce the same character on both layouts.
const sharedChars = new Set([
  ...'0123456789',
  '!',
  '%',
  '*',
  '(',
  ')',
  '-',
  '_',
  '=',
  '+',
  '\\',
  ' ',
  '\n',
  '\t'
]);

// Characters the paste API can type on a US layout.
const enChars = new Set(
  `abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*() \n\t-=[]\\;'\`,./_+{}|:"~<>?`
);

const letterMap: Record<string, string> = {
  ё: '`',
  й: 'q',
  ц: 'w',
  у: 'e',
  к: 'r',
  е: 't',
  н: 'y',
  г: 'u',
  ш: 'i',
  щ: 'o',
  з: 'p',
  х: '[',
  ъ: ']',
  ф: 'a',
  ы: 's',
  в: 'd',
  а: 'f',
  п: 'g',
  р: 'h',
  о: 'j',
  л: 'k',
  д: 'l',
  ж: ';',
  э: "'",
  я: 'z',
  ч: 'x',
  с: 'c',
  м: 'v',
  и: 'b',
  т: 'n',
  ь: 'm',
  б: ',',
  ю: '.'
};

const punctuationMap: Record<string, string> = {
  '"': '@',
  '№': '#',
  ';': '$',
  ':': '^',
  '?': '&',
  '/': '|',
  '.': '/',
  ',': '?'
};

const shiftMap: Record<string, string> = {
  '`': '~',
  '-': '_',
  '=': '+',
  '[': '{',
  ']': '}',
  '\\': '|',
  ';': ':',
  "'": '"',
  ',': '<',
  '.': '>',
  '/': '?'
};

function withShift(key: string): string {
  if (key >= 'a' && key <= 'z') return key.toUpperCase();
  return shiftMap[key] ?? key;
}

function enKeyFor(ch: string): string | null {
  return enChars.has(ch) ? ch : null;
}

function ruKeyFor(ch: string): string | null {
  if (sharedChars.has(ch)) return ch;

  const lower = ch.toLowerCase();
  const base = letterMap[lower];
  if (base) return ch === lower ? base : withShift(base);

  return punctuationMap[ch] ?? null;
}

function keyFor(layout: Layout, ch: string): string | null {
  return layout === 'en' ? enKeyFor(ch) : ruKeyFor(ch);
}

function canType(value: string): boolean {
  for (const ch of value) {
    if (ch === '\r') continue;
    if (enKeyFor(ch) == null && ruKeyFor(ch) == null) return false;
  }
  return true;
}

type PasteSegment = { layout: Layout; text: string };

// Split text into layout runs. A run changes only when the current layout
// cannot type the character and the other layout can.
function planPaste(value: string, start: Layout): PasteSegment[] | null {
  let layout = start;
  const segments: PasteSegment[] = [];
  let text = '';

  const flush = () => {
    if (!text) return;
    segments.push({ layout, text });
    text = '';
  };

  for (const ch of value) {
    if (ch === '\r') continue;

    const key = keyFor(layout, ch);
    if (key != null) {
      text += key;
      continue;
    }

    const other: Layout = layout === 'en' ? 'ru' : 'en';
    const otherKey = keyFor(other, ch);
    if (otherKey == null) return null;

    flush();
    layout = other;
    text = otherKey;
  }

  flush();
  return segments;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function sendKeyboard(report: Uint8Array) {
  return client.send(new Uint8Array([MessageEvent.Keyboard, ...report]));
}

async function pressKeys(codes: string[], holdMs: number) {
  const keyboard = new KeyboardReport();

  for (const code of codes) {
    if (!sendKeyboard(keyboard.keyDown(code))) return false;
    await sleep(30);
  }

  await sleep(holdMs);
  return sendKeyboard(keyboard.reset());
}

export const Paste = () => {
  const { t } = useTranslation();
  const setIsKeyboardEnable = useSetAtom(isKeyboardEnableAtom);

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [language, setLanguage] = useState<Layout>('en');
  const [status, setStatus] = useState<'' | 'error'>('');
  const [isLoading, setIsLoading] = useState(false);
  const [sendingKey, setSendingKey] = useState<'' | 'alt' | 'enter'>('');
  const [errMsg, setErrMsg] = useState('');

  const inputRef = useRef<InputRef>(null);
  const sendingRef = useRef(false);

  function onChange(e: ChangeEvent<HTMLTextAreaElement>) {
    const value = e.target.value;
    setStatus(canType(value) ? '' : 'error');
    setInputValue(value);
  }

  function onLanguageChange(value: Layout) {
    setLanguage(value);
  }

  async function submit() {
    if (isLoading || sendingRef.current || !inputValue || status === 'error') return;

    const segments = planPaste(inputValue, language);
    if (!segments) {
      setStatus('error');
      return;
    }

    sendingRef.current = true;
    setIsLoading(true);
    let layout = language;

    try {
      for (const segment of segments) {
        if (segment.layout !== layout) {
          const switched = await pressKeys(['AltLeft', 'ShiftLeft'], 50);
          if (!switched) {
            setErrMsg(t('keyboard.switchFailed'));
            return;
          }
          await sleep(100);
          layout = segment.layout;
          setLanguage(layout);
        }

        const rsp = await paste(segment.text);
        if (rsp.code !== 0) {
          setErrMsg(rsp.msg);
          return;
        }
      }

      setInputValue('');
      setStatus('');
      setErrMsg('');
      setIsModalOpen(false);
    } finally {
      sendingRef.current = false;
      setIsLoading(false);
    }
  }

  async function sendKeys(kind: 'alt' | 'enter', codes: string[], holdMs: number) {
    if (sendingRef.current) return;
    sendingRef.current = true;
    setSendingKey(kind);
    try {
      const sent = await pressKeys(codes, holdMs);
      if (!sent) {
        setErrMsg(t('keyboard.switchFailed'));
        return;
      }
      if (kind === 'alt') {
        setLanguage((current) => (current === 'en' ? 'ru' : 'en'));
      }
    } finally {
      sendingRef.current = false;
      setSendingKey('');
    }
  }

  function afterOpenChange(open: boolean) {
    if (open) {
      inputRef.current?.focus();
    }
    setIsKeyboardEnable(!open);
  }

  return (
    <>
      <div
        className={clsx(
          'flex cursor-pointer select-none items-center space-x-2 rounded py-1 pl-2 pr-5 hover:bg-neutral-700/70'
        )}
        onClick={() => setIsModalOpen(true)}
      >
        <ClipboardIcon size={18} />
        <span>{t('keyboard.paste')}</span>
      </div>

      <Modal
        open={isModalOpen}
        centered={false}
        title={t('keyboard.paste')}
        footer={null}
        onCancel={() => setIsModalOpen(false)}
        afterOpenChange={afterOpenChange}
      >
        <div className="flex items-center justify-between gap-2 pb-2">
          <div className="flex items-center gap-2">
            <span className="text-sm text-neutral-600">{t('keyboard.virtual')}:</span>
            <Select
              size="small"
              style={{ minWidth: 120 }}
              value={language}
              options={languages.map((l) => ({ value: l.value, label: t(l.labelKey) }))}
              onChange={onLanguageChange}
            />
          </div>
          <div className="flex items-center gap-2">
            <Tooltip title={t('keyboard.altShiftTip')}>
              <Button
                size="small"
                loading={sendingKey === 'alt'}
                disabled={isLoading || sendingKey === 'enter'}
                onClick={() => sendKeys('alt', ['AltLeft', 'ShiftLeft'], 50)}
              >
                Alt+Shift
              </Button>
            </Tooltip>
            <Tooltip title={t('keyboard.enterTip')}>
              <Button
                size="small"
                loading={sendingKey === 'enter'}
                disabled={isLoading || sendingKey === 'alt'}
                onClick={() => sendKeys('enter', ['Enter'], 40)}
              >
                Enter
              </Button>
            </Tooltip>
          </div>
        </div>
        <div className="pb-3 text-xs text-neutral-500">{t('keyboard.tips')}</div>

        <TextArea
          ref={inputRef}
          value={inputValue}
          status={status}
          showCount
          maxLength={1024}
          autoSize={{ minRows: 5, maxRows: 12 }}
          placeholder={t('keyboard.placeholder')}
          onChange={onChange}
        />

        {errMsg && <div className="pt-1 text-sm text-red-500">{errMsg}</div>}

        <div className="flex justify-center py-3">
          <Button type="primary" loading={isLoading} disabled={status === 'error'} htmlType="submit" onClick={submit}>
            {t('keyboard.submit')}
          </Button>
        </div>
      </Modal>
    </>
  );
};
