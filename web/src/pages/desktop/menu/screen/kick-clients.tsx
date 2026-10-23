import { useState } from 'react';
import { Popconfirm } from 'antd';
import { useAtomValue } from 'jotai';
import { UserXIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/stream.ts';
import { videoModeAtom } from '@/jotai/screen.ts';

export const KickClients = () => {
  const { t } = useTranslation();
  const videoMode = useAtomValue(videoModeAtom);
  const [isLoading, setIsLoading] = useState(false);

  async function kick() {
    if (isLoading) return;
    setIsLoading(true);

    try {
      await api.kickClients(videoMode || undefined);
    } catch (err) {
      console.error(err);
    } finally {
      window.location.reload();
    }
  }

  return (
    <Popconfirm
      placement="rightTop"
      title={t('screen.kickClientsConfirm')}
      okText={t('power.okBtn')}
      cancelText={t('power.cancelBtn')}
      onConfirm={kick}
      color="#404040"
    >
      <div className="flex cursor-pointer select-none items-center rounded py-1.5 pl-1 pr-5 text-neutral-300 hover:bg-neutral-700/70">
        <div className="flex h-[14px] w-[20px] items-end">
          <UserXIcon size={15} />
        </div>
        <span>{t('screen.kickClients')}</span>
      </div>
    </Popconfirm>
  );
};
