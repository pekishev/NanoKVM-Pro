import { useEffect, useRef, useState } from 'react';
import { Button, Divider, Input, Modal } from 'antd';
import { useSetAtom } from 'jotai';
import { Trash2Icon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/hid.ts';
import { captureScreenFrame } from '@/lib/screen-capture.ts';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { ScrollArea } from '@/components/ui/scroll-area.tsx';

import { type MacroImagePayload, cropToPng } from './image.ts';
import { CoordinatePicker } from './picker.tsx';
import { applyRecordEvent, createRecordState } from './record.ts';
import { RegionPicker } from './region.tsx';
import { MACRO_IMAGE_NAME, MACRO_IMAGE_TIMEOUT_MAX, parseScript } from './script.ts';
import type { Macro, ScreenRect } from './types.ts';

interface EditorProps {
  macros: Macro[];
  saveMacro: (macro: Macro) => Promise<string | null>;
  delMacro: (macro: Macro) => Promise<void>;
  reloadMacros: () => Promise<void>;
  setIsEditing: (isEditing: boolean) => void;
  setIsPicking: (isPicking: boolean) => void;
}

type InsertKind = 'CLICK' | 'DBLCLICK' | 'MOVE' | 'IMAGE';

const emptyMacro = (): Macro => ({ name: '', script: '' });

export const Editor = ({
  macros,
  saveMacro,
  delMacro,
  reloadMacros,
  setIsEditing,
  setIsPicking
}: EditorProps) => {
  const { t } = useTranslation();
  const setIsKeyboardEnable = useSetAtom(isKeyboardEnableAtom);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  const [isModalOpen, setIsModalOpen] = useState(false);
  const [draft, setDraft] = useState<Macro>(emptyMacro);
  const [pickingKind, setPickingKind] = useState<InsertKind | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [parseError, setParseError] = useState('');
  const [isRecording, setIsRecording] = useState(false);
  const [recordStatus, setRecordStatus] = useState('');
  const [imageAsk, setImageAsk] = useState(false);
  const [imageName, setImageName] = useState('');
  const [imageTimeout, setImageTimeout] = useState('20000');
  const [pendingImages, setPendingImages] = useState<Record<string, MacroImagePayload>>({});
  const recordStateRef = useRef(createRecordState());
  const fullscreenRequestedRef = useRef(false);
  const imagePickRef = useRef<{ name: string; ms: number } | null>(null);

  const isPicking = pickingKind !== null;
  const parsed = parseScript(draft.script);
  const canSave = !!draft.name.trim() && parsed.ok;

  useEffect(() => {
    setIsKeyboardEnable(!isModalOpen && !isPicking);
    setIsEditing(isModalOpen);
    setIsPicking(isPicking);
  }, [isModalOpen, isPicking, setIsEditing, setIsKeyboardEnable, setIsPicking]);

  useEffect(() => {
    if (!isRecording) return;

    const onKeyDown = (event: KeyboardEvent) => {
      event.preventDefault();
      event.stopPropagation();
      const result = applyRecordEvent(recordStateRef.current, 'keydown', event.code, event.repeat);
      if (!result) return;
      if ('line' in result) {
        appendRecordedLine(result.line);
        setRecordStatus(result.line);
        return;
      }
      setRecordStatus(t('keyboard.macro.recordUnknown'));
    };

    const onKeyUp = (event: KeyboardEvent) => {
      event.preventDefault();
      event.stopPropagation();
      const result = applyRecordEvent(recordStateRef.current, 'keyup', event.code, false);
      if (result && 'line' in result) {
        appendRecordedLine(result.line);
        setRecordStatus(result.line);
      }
    };

    window.addEventListener('keydown', onKeyDown, true);
    window.addEventListener('keyup', onKeyUp, true);
    return () => {
      window.removeEventListener('keydown', onKeyDown, true);
      window.removeEventListener('keyup', onKeyUp, true);
    };
  }, [isRecording, t]);

  function openModal(macro?: Macro) {
    stopRecording();
    setDraft(macro ? { ...macro } : emptyMacro());
    setPickingKind(null);
    setParseError('');
    setImageAsk(false);
    setImageName('');
    setImageTimeout('20000');
    setPendingImages({});
    imagePickRef.current = null;
    setIsModalOpen(true);
  }

  function stopRecording() {
    setIsRecording(false);
    setRecordStatus('');
    recordStateRef.current = createRecordState();

    const keyboardApi = (navigator as Navigator & { keyboard?: { unlock?: () => void } }).keyboard;
    keyboardApi?.unlock?.();

    if (fullscreenRequestedRef.current && document.fullscreenElement) {
      fullscreenRequestedRef.current = false;
      document.exitFullscreen().catch(() => undefined);
    }
  }

  function closeModal() {
    if (isPicking) return;
    stopRecording();
    setDraft(emptyMacro());
    setParseError('');
    setImageAsk(false);
    setPendingImages({});
    imagePickRef.current = null;
    setIsModalOpen(false);
  }

  function insertSnippet(snippet: string, withNewline = true) {
    const el = textareaRef.current;
    const value = draft.script;
    const start = el?.selectionStart ?? value.length;
    const end = el?.selectionEnd ?? start;
    const before = value.slice(0, start);
    const after = value.slice(end);
    const prefix = before && !before.endsWith('\n') ? '\n' : '';
    const suffix = withNewline && after && !after.startsWith('\n') ? '\n' : '';
    const next = `${before}${prefix}${snippet}${suffix}${after}`;
    const cursor = before.length + prefix.length + snippet.length;

    setDraft((prev) => ({ ...prev, script: next }));
    setParseError('');

    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(cursor, cursor);
    });
  }

  function appendRecordedLine(line: string) {
    setDraft((prev) => {
      const script = prev.script;
      const prefix = script && !script.endsWith('\n') ? '\n' : '';
      return { ...prev, script: `${script}${prefix}${line}\n` };
    });
    setParseError('');
  }

  function toggleRecording() {
    if (isRecording) {
      stopRecording();
      return;
    }

    recordStateRef.current = createRecordState();
    setRecordStatus('');
    setIsRecording(true);
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }
  }

  async function enableSystemKeys() {
    if (!document.fullscreenElement) {
      fullscreenRequestedRef.current = true;
      await document.documentElement.requestFullscreen();
    }
    const keyboardApi = (navigator as Navigator & { keyboard?: { lock?: () => Promise<void> } }).keyboard;
    await keyboardApi?.lock?.();
  }

  function startPicking(kind: InsertKind) {
    stopRecording();
    setPickingKind(kind);
    setIsModalOpen(false);
  }

  function handlePicked(x: number, y: number) {
    if (!pickingKind) return;
    const line = `${pickingKind} ${x} ${y}`;
    setDraft((prev) => {
      const script = prev.script.trimEnd();
      const prefix = script ? `${script}\n` : '';
      return { ...prev, script: `${prefix}${line}\n` };
    });
    setPickingKind(null);
    setIsModalOpen(true);
  }

  function cancelPicking() {
    imagePickRef.current = null;
    setPickingKind(null);
    setIsModalOpen(true);
  }

  function startImagePick() {
    const name = imageName.trim();
    const ms = Number(imageTimeout);
    if (!MACRO_IMAGE_NAME.test(name)) {
      setParseError(t('keyboard.macro.imageNameInvalid'));
      return;
    }
    if (!Number.isInteger(ms) || ms < 0 || ms > MACRO_IMAGE_TIMEOUT_MAX) {
      setParseError(t('keyboard.macro.imageTimeoutInvalid'));
      return;
    }

    stopRecording();
    imagePickRef.current = { name, ms };
    setParseError('');
    setPickingKind('IMAGE');
    setIsModalOpen(false);
  }

  async function handleRegion(rect: ScreenRect) {
    const pick = imagePickRef.current;
    imagePickRef.current = null;
    setPickingKind(null);
    setIsModalOpen(true);
    if (!pick) return;

    const frame = await captureScreenFrame();
    if (!frame) {
      setParseError(t('keyboard.macro.imageCaptureFailed'));
      return;
    }

    const png = cropToPng(frame, rect);
    if (!png) {
      setParseError(t('keyboard.macro.imageCaptureFailed'));
      return;
    }

    setPendingImages((prev) => ({
      ...prev,
      [pick.name]: { name: pick.name, x: rect.x, y: rect.y, w: rect.w, h: rect.h, png }
    }));
    setDraft((prev) => {
      const script = prev.script.trimEnd();
      const prefix = script ? `${script}\n` : '';
      return { ...prev, script: `${prefix}WAITIMAGE ${pick.name} ${pick.ms}\n` };
    });
    setParseError('');
  }

  async function handleSave() {
    const result = parseScript(draft.script);
    if (isSaving || !draft.name.trim() || !result.ok) {
      if (!result.ok) {
        setParseError(t('keyboard.macro.parseError', { line: result.line, error: result.error }));
      }
      return;
    }

    setIsSaving(true);
    try {
      const savedId = await saveMacro({
        id: draft.id,
        name: draft.name.trim(),
        script: draft.script.replace(/\r\n/g, '\n')
      });
      if (!savedId) return;

      setDraft((prev) => ({ ...prev, id: savedId }));
      const images = Object.values(pendingImages);
      for (const image of images) {
        const rsp = await api.saveMacroImage({ id: savedId, ...image });
        if (rsp.code !== 0) {
          await reloadMacros();
          setParseError(t('keyboard.macro.imageSaveFailed'));
          return;
        }
      }
      await reloadMacros();

      stopRecording();
      setDraft(emptyMacro());
      setPendingImages({});
      setImageAsk(false);
      setParseError('');
    } catch (err) {
      console.log(err);
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <>
      <div
        className="flex h-[30px] cursor-pointer items-center space-x-1 rounded px-3 text-neutral-300 hover:bg-neutral-700/60"
        onClick={() => openModal()}
      >
        <span>{t('keyboard.macro.custom')}</span>
      </div>

      <Modal
        width={640}
        title={draft.id ? t('keyboard.macro.edit') : t('keyboard.macro.title')}
        keyboard={false}
        footer={null}
        open={isModalOpen}
        onCancel={closeModal}
      >
        <div className="flex flex-col space-y-3">
          <Input
            maxLength={64}
            placeholder={t('keyboard.macro.namePlaceholder')}
            value={draft.name}
            onChange={(e) => setDraft((prev) => ({ ...prev, name: e.target.value }))}
          />

          <pre className="whitespace-pre-wrap rounded bg-neutral-800/70 px-3 py-2 text-xs leading-5 text-neutral-400">
            {t('keyboard.macro.tips')}
          </pre>

          <div className="flex flex-wrap gap-2">
            <Button size="small" type={isRecording ? 'primary' : 'default'} danger={isRecording} onClick={toggleRecording}>
              {isRecording ? t('keyboard.macro.stopRecord') : t('keyboard.macro.record')}
            </Button>
            <Button size="small" onClick={() => insertSnippet('STRING ', false)}>
              STRING
            </Button>
            <Button size="small" onClick={() => insertSnippet('ENTER')}>
              ENTER
            </Button>
            <Button size="small" onClick={() => insertSnippet('TAB')}>
              TAB
            </Button>
            <Button size="small" onClick={() => insertSnippet('WIN+SHIFT+S')}>
              WIN+SHIFT+S
            </Button>
            <Button size="small" onClick={() => insertSnippet('CTRL+ALT+DELETE')}>
              CTRL+ALT+DEL
            </Button>
            <Button size="small" onClick={() => insertSnippet('DELAY 200')}>
              DELAY
            </Button>
            <Button size="small" onClick={() => insertSnippet('RUN ', false)}>
              RUN
            </Button>
            <Button size="small" onClick={() => startPicking('CLICK')}>
              CLICK
            </Button>
            <Button size="small" onClick={() => startPicking('DBLCLICK')}>
              DBLCLICK
            </Button>
            <Button size="small" onClick={() => startPicking('MOVE')}>
              MOVE
            </Button>
            <Button size="small" type={imageAsk ? 'primary' : 'default'} onClick={() => setImageAsk((open) => !open)}>
              WAITIMAGE
            </Button>
          </div>

          {imageAsk && (
            <div className="flex flex-wrap items-center gap-2">
              <Input
                size="small"
                maxLength={32}
                className="w-36"
                placeholder={t('keyboard.macro.imageName')}
                value={imageName}
                onChange={(e) => setImageName(e.target.value)}
              />
              <Input
                size="small"
                className="w-28"
                placeholder="20000"
                value={imageTimeout}
                onChange={(e) => setImageTimeout(e.target.value)}
              />
              <Button size="small" type="primary" onClick={startImagePick}>
                {t('keyboard.macro.imageSelect')}
              </Button>
              <span className="text-xs text-neutral-500">{t('keyboard.macro.imageHint')}</span>
            </div>
          )}

          {Object.keys(pendingImages).length > 0 && (
            <div className="text-xs text-neutral-400">
              {t('keyboard.macro.imagePending', { names: Object.keys(pendingImages).join(', ') })}
            </div>
          )}

          {isRecording && (
            <div className="text-xs leading-5 text-neutral-400">
              <div>{recordStatus || t('keyboard.macro.recording')}</div>
              <button type="button" className="text-blue-400" onClick={() => enableSystemKeys()}>
                {t('keyboard.macro.recordFullscreen')}
              </button>
            </div>
          )}

          <textarea
            ref={textareaRef}
            spellCheck={false}
            value={draft.script}
            placeholder={t('keyboard.macro.placeholder')}
            className="h-[280px] w-full resize-y rounded border border-neutral-700 bg-neutral-950 p-3 font-mono text-sm leading-6 text-neutral-200 outline-none placeholder:text-neutral-600 focus:border-neutral-500"
            onChange={(e) => {
              setDraft((prev) => ({ ...prev, script: e.target.value }));
              setParseError('');
            }}
          />

          {(parseError || (!parsed.ok && draft.script.trim())) && (
            <div className="text-sm text-red-400">
              {parseError ||
                t('keyboard.macro.parseError', { line: parsed.ok ? 0 : parsed.line, error: parsed.ok ? '' : parsed.error })}
            </div>
          )}

          <div className="flex justify-center pb-1">
            <Button type="primary" className="min-w-24" loading={isSaving} disabled={!canSave} onClick={handleSave}>
              {t('keyboard.macro.save')}
            </Button>
          </div>
        </div>

        {macros.length > 0 && (
          <>
            <Divider />
            <ScrollArea className="[&>[data-radix-scroll-area-viewport]]:max-h-[220px]">
              {macros.map((macro) => (
                <div
                  key={macro.id}
                  className="flex items-center justify-between rounded p-2 hover:bg-neutral-700/50"
                >
                  <div className="min-w-0">
                    <div className="truncate">{macro.name}</div>
                    <div className="truncate font-mono text-xs text-neutral-500">
                      {macro.script.trim().split('\n')[0]}
                    </div>
                  </div>
                  <div className="flex items-center space-x-2">
                    <Button size="small" onClick={() => openModal(macro)}>
                      {t('keyboard.macro.edit')}
                    </Button>
                    <div
                      className="flex size-[20px] cursor-pointer items-center justify-center rounded-sm text-neutral-500 hover:text-red-500"
                      onClick={() => macro.id && delMacro(macro)}
                    >
                      <Trash2Icon size={16} />
                    </div>
                  </div>
                </div>
              ))}
            </ScrollArea>
          </>
        )}
      </Modal>

      <CoordinatePicker open={isPicking && pickingKind !== 'IMAGE'} onPick={handlePicked} onCancel={cancelPicking} />
      <RegionPicker open={pickingKind === 'IMAGE'} onPick={handleRegion} onCancel={cancelPicking} />
    </>
  );
};
