import { useEffect } from 'react';
import { useAtom } from 'jotai';

import * as api from '@/api/vm.ts';
import { hdmiAudioEnabledAtom, hdmiAudioLoadedAtom } from '@/jotai/audio.ts';

export function useHdmiAudio() {
  const [enabled, setEnabled] = useAtom(hdmiAudioEnabledAtom);
  const [loaded, setLoaded] = useAtom(hdmiAudioLoadedAtom);

  useEffect(() => {
    let cancelled = false;

    api
      .getHdmiAudio()
      .then((rsp) => {
        if (cancelled || rsp.code !== 0) {
          return;
        }
        setEnabled(!!rsp.data?.enabled);
      })
      .finally(() => {
        if (!cancelled) {
          setLoaded(true);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [setEnabled, setLoaded]);

  return { enabled, loaded, setEnabled };
}
