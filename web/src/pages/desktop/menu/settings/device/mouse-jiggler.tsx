import { useEffect, useState } from 'react';
import { Switch } from 'antd';
import { useTranslation } from 'react-i18next';

import * as api from '@/api/vm.ts';

export const MouseJiggler = () => {
  const { t } = useTranslation();

  const [enabled, setEnabled] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    getMouseJiggler();
  }, []);

  function getMouseJiggler() {
    setIsLoading(true);

    api
      .getMouseJiggler()
      .then((rsp) => {
        if (rsp.code !== 0) {
          console.log(rsp.msg);
          return;
        }

        setEnabled(rsp.data.enabled);
      })
      .finally(() => {
        setIsLoading(false);
      });
  }

  function update(checked: boolean) {
    if (isLoading) return;
    setIsLoading(true);

    api
      .setMouseJiggler(checked)
      .then((rsp) => {
        if (rsp.code !== 0) {
          console.log(rsp.msg);
          return;
        }

        setEnabled(checked);
      })
      .finally(() => {
        setIsLoading(false);
      });
  }

  return (
    <div className="flex items-center justify-between">
      <div className="flex flex-col space-y-1">
        <span>{t('settings.device.mouseJiggler.title')}</span>
        <span className="text-xs text-neutral-500">
          {t('settings.device.mouseJiggler.description')}
        </span>
      </div>

      <Switch checked={enabled} loading={isLoading} onChange={update} />
    </div>
  );
};
