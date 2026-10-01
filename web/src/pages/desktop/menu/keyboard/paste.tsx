import { ChangeEvent, useEffect, useRef, useState } from 'react';
import { Button, Input, Modal, Select, Space, Tooltip, type InputRef } from 'antd';
import clsx from 'clsx';
import { useSetAtom } from 'jotai';
import { AudioLinesIcon, ClipboardIcon, MicIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { paste, recognizeSpeech } from '@/api/hid';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { KeyboardReport } from '@/lib/keyboard.ts';
import { startWavRecorder, type WavRecorder } from '@/lib/wav-recorder.ts';
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

function isTypeable(ch: string): boolean {
  return enKeyFor(ch) != null || ruKeyFor(ch) != null;
}

function canType(value: string): boolean {
  for (const ch of value) {
    if (ch === '\r') continue;
    if (!isTypeable(ch)) return false;
  }
  return true;
}

const maxLength = 1024;

// Typographic characters that speech recognizers emit but a keyboard cannot type.
const speechReplacements: Record<string, string> = {
  '«': '"',
  '»': '"',
  '„': '"',
  '“': '"',
  '”': '"',
  '‘': "'",
  '’': "'",
  '‚': "'",
  '—': '-',
  '–': '-',
  '‒': '-',
  '−': '-',
  '‑': '-',
  '…': '...',
  '\u00a0': ' ',
  '\u2009': ' ',
  '\u202f': ' '
};

function toTypeable(text: string): string {
  let out = '';
  for (const ch of text) {
    for (const c of speechReplacements[ch] ?? ch) {
      if (isTypeable(c)) out += c;
    }
  }
  return out;
}

function appendChunk(text: string, chunk: string): string {
  const trimmed = chunk.trim();
  if (!trimmed) return text;
  return text && !/\s$/.test(text) ? `${text} ${trimmed}` : text + trimmed;
}

function normalizeSpeech(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, ' ')
    .trim();
}

// Some engines (e.g. Chrome on Android) emit results that repeat the whole
// phrase so far, so a result extending the previous one replaces it.
function joinSpeechResults(results: ArrayLike<SpeechRecognitionResultLike>): string {
  const pieces: string[] = [];
  for (let i = 0; i < results.length; i++) {
    const text = toTypeable(results[i][0].transcript).trim();
    const norm = normalizeSpeech(text);
    if (!norm) continue;

    const prev = pieces.length ? normalizeSpeech(pieces[pieces.length - 1]) : '';
    if (prev && (norm === prev || norm.startsWith(`${prev} `))) {
      pieces[pieces.length - 1] = text;
    } else {
      pieces.push(text);
    }
  }
  return pieces.reduce(appendChunk, '');
}

type SpeechRecognitionResultLike = { isFinal: boolean; 0: { transcript: string } };

interface SpeechRecognitionLike {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  onresult: ((e: { resultIndex: number; results: ArrayLike<SpeechRecognitionResultLike> }) => void) | null;
  onerror: ((e: { error: string }) => void) | null;
  onend: (() => void) | null;
  start(): void;
  stop(): void;
  abort(): void;
}

type SpeechRecognitionCtor = new () => SpeechRecognitionLike;

const speechWindow = window as unknown as {
  SpeechRecognition?: SpeechRecognitionCtor;
  webkitSpeechRecognition?: SpeechRecognitionCtor;
};
const SpeechRecognitionImpl = speechWindow.SpeechRecognition ?? speechWindow.webkitSpeechRecognition;

const voiceLanguages = [
  { value: 'ru', label: 'RU' },
  { value: 'en', label: 'EN' }
];

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
  const { t, i18n } = useTranslation();
  const setIsKeyboardEnable = useSetAtom(isKeyboardEnableAtom);

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [language, setLanguage] = useState<Layout>('en');
  const [status, setStatus] = useState<'' | 'error'>('');
  const [isLoading, setIsLoading] = useState(false);
  const [sendingKey, setSendingKey] = useState<'' | 'alt' | 'enter'>('');
  const [errMsg, setErrMsg] = useState('');

  const [voiceLanguage, setVoiceLanguage] = useState<Layout>(i18n.language.startsWith('ru') ? 'ru' : 'en');
  const [isListening, setIsListening] = useState(false);

  const inputRef = useRef<InputRef>(null);
  const sendingRef = useRef(false);
  const recognitionRef = useRef<SpeechRecognitionLike | null>(null);
  const voiceTextRef = useRef('');

  const [localVoice, setLocalVoice] = useState<'' | 'recording' | 'processing'>('');
  const recorderRef = useRef<WavRecorder | null>(null);
  const recorderStartingRef = useRef(false);
  const localVoiceSeqRef = useRef(0);

  useEffect(
    () => () => {
      recognitionRef.current?.abort();
      recorderRef.current?.cancel();
    },
    []
  );

  function cancelLocalVoice() {
    localVoiceSeqRef.current++;
    recorderRef.current?.cancel();
    recorderRef.current = null;
    setLocalVoice('');
  }

  async function toggleLocalVoice() {
    const recorder = recorderRef.current;
    if (recorder) {
      recorderRef.current = null;
      const seq = ++localVoiceSeqRef.current;
      const { wav, silent } = recorder.stop();
      if (silent) {
        setLocalVoice('');
        setErrMsg(t('keyboard.voiceSilent', { device: recorder.device || '?' }));
        return;
      }
      setLocalVoice('processing');
      try {
        const rsp = await recognizeSpeech(wav);
        if (seq !== localVoiceSeqRef.current) return;
        if (rsp.code !== 0) {
          setErrMsg(t('keyboard.voiceLocalFailed', { error: rsp.msg }));
          return;
        }
        const value = appendChunk(inputValue, toTypeable(rsp.data?.text ?? '')).slice(0, maxLength);
        setStatus(canType(value) ? '' : 'error');
        setInputValue(value);
      } catch (err) {
        if (seq === localVoiceSeqRef.current) setErrMsg(t('keyboard.voiceLocalFailed', { error: String(err) }));
      } finally {
        if (seq === localVoiceSeqRef.current) setLocalVoice('');
      }
      return;
    }

    if (recorderStartingRef.current) return;
    recorderStartingRef.current = true;
    setErrMsg('');
    try {
      recorderRef.current = await startWavRecorder();
      setLocalVoice('recording');
    } catch (err) {
      const denied = err instanceof DOMException && err.name === 'NotAllowedError';
      setErrMsg(denied ? t('keyboard.voiceDenied') : t('keyboard.voiceLocalFailed', { error: String(err) }));
    } finally {
      recorderStartingRef.current = false;
    }
  }

  function stopVoice() {
    const recognition = recognitionRef.current;
    if (!recognition) return;
    recognitionRef.current = null;
    recognition.onresult = null;
    recognition.abort();
    setIsListening(false);
  }

  function voiceErrorMessage(error: string): string {
    if (error === 'not-allowed' || error === 'service-not-allowed') return t('keyboard.voiceDenied');
    if (error === 'network') return t('keyboard.voiceNetwork');
    return t('keyboard.voiceError', { error });
  }

  function toggleVoice() {
    if (recognitionRef.current) {
      recognitionRef.current.stop();
      return;
    }
    if (!SpeechRecognitionImpl) return;

    const recognition = new SpeechRecognitionImpl();
    recognition.lang = voiceLanguage === 'ru' ? 'ru-RU' : 'en-US';
    recognition.continuous = true;
    recognition.interimResults = true;
    voiceTextRef.current = inputValue;

    recognition.onresult = (e) => {
      const value = appendChunk(voiceTextRef.current, joinSpeechResults(e.results)).slice(0, maxLength);
      setStatus(canType(value) ? '' : 'error');
      setInputValue(value);
    };
    recognition.onerror = (e) => {
      if (e.error === 'no-speech' || e.error === 'aborted') return;
      setErrMsg(voiceErrorMessage(e.error));
    };
    recognition.onend = () => {
      if (recognitionRef.current !== recognition) return;
      recognitionRef.current = null;
      setIsListening(false);
    };

    setErrMsg('');
    recognitionRef.current = recognition;
    try {
      recognition.start();
      setIsListening(true);
    } catch (err) {
      recognitionRef.current = null;
      setErrMsg(voiceErrorMessage(String(err)));
    }
  }

  function closeModal() {
    stopVoice();
    cancelLocalVoice();
    setIsModalOpen(false);
  }

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

    stopVoice();
    cancelLocalVoice();
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
        onCancel={closeModal}
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
          maxLength={maxLength}
          autoSize={{ minRows: 5, maxRows: 12 }}
          placeholder={t('keyboard.placeholder')}
          readOnly={isListening || localVoice !== ''}
          onChange={onChange}
        />

        {errMsg && <div className="pt-1 text-sm text-red-500">{errMsg}</div>}

        <div className="flex items-center justify-center gap-2 py-3">
          {SpeechRecognitionImpl && (
            <Space.Compact>
              <Select
                value={voiceLanguage}
                options={voiceLanguages}
                disabled={isListening}
                onChange={setVoiceLanguage}
              />
              <Tooltip
                title={
                  !window.isSecureContext
                    ? t('keyboard.voiceHttps')
                    : isListening
                      ? t('keyboard.voiceStop')
                      : t('keyboard.voice')
                }
              >
                <Button
                  icon={<MicIcon size={16} className={clsx(isListening && 'animate-pulse')} />}
                  danger={isListening}
                  disabled={isLoading || localVoice !== '' || !window.isSecureContext}
                  onClick={toggleVoice}
                />
              </Tooltip>
            </Space.Compact>
          )}
          <Tooltip
            title={
              !window.isSecureContext
                ? t('keyboard.voiceHttps')
                : localVoice === 'recording'
                  ? t('keyboard.voiceLocalStop')
                  : t('keyboard.voiceLocal')
            }
          >
            <Button
              icon={<AudioLinesIcon size={16} className={clsx(localVoice === 'recording' && 'animate-pulse')} />}
              danger={localVoice === 'recording'}
              loading={localVoice === 'processing'}
              disabled={isLoading || isListening || !window.isSecureContext}
              onClick={toggleLocalVoice}
            />
          </Tooltip>
          <Button
            type="primary"
            loading={isLoading}
            disabled={status === 'error' || localVoice === 'processing'}
            htmlType="submit"
            onClick={submit}
          >
            {t('keyboard.submit')}
          </Button>
        </div>
      </Modal>
    </>
  );
};
