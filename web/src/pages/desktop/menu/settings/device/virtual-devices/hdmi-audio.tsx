import { useEffect, useState } from 'react';
import { Switch, Tooltip } from 'antd';
import { useSetAtom } from 'jotai';
import { CircleAlertIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/vm.ts';
import { hdmiAudioEnabledAtom } from '@/jotai/audio.ts';

export const HdmiAudio = () => {
  const { t } = useTranslation();
  const setHdmiAudioEnabled = useSetAtom(hdmiAudioEnabledAtom);

  const [isEnabled, setIsEnabled] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    setIsLoading(true);
    api
      .getHdmiAudio()
      .then((rsp) => {
        if (rsp.code !== 0) {
          return;
        }
        const enabled = !!rsp.data?.enabled;
        setIsEnabled(enabled);
        setHdmiAudioEnabled(enabled);
      })
      .finally(() => {
        setIsLoading(false);
      });
  }, [setHdmiAudioEnabled]);

  function update() {
    if (isLoading) return;
    setIsLoading(true);

    const next = !isEnabled;
    api
      .setHdmiAudio(next)
      .then((rsp) => {
        if (rsp.code !== 0) {
          return;
        }
        setIsEnabled(next);
        setHdmiAudioEnabled(next);
        // Rebuild WebRTC without/with the audio peer connection.
        window.location.reload();
      })
      .finally(() => {
        setIsLoading(false);
      });
  }

  return (
    <div className="flex items-center justify-between">
      <div className="flex flex-col space-y-1">
        <div className="flex items-center space-x-2">
          <span>{t('settings.device.hdmiAudio')}</span>
          <Tooltip
            title={t('settings.device.hdmiAudioTip')}
            className="cursor-pointer text-neutral-500"
            placement="top"
            styles={{ root: { maxWidth: '450px', whiteSpace: 'pre-wrap' } }}
          >
            <CircleAlertIcon size={15} />
          </Tooltip>
        </div>
        <span className="text-xs text-neutral-500">{t('settings.device.hdmiAudioDesc')}</span>
      </div>

      <Switch checked={isEnabled} loading={isLoading} onChange={update} />
    </div>
  );
};
