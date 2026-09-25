import { ChangeEvent, useEffect, useRef, useState } from 'react';
import { LoadingOutlined, RocketOutlined, SmileOutlined, UploadOutlined } from '@ant-design/icons';
import { Button, Divider, Result, Spin } from 'antd';
import { useTranslation } from 'react-i18next';
import semver from 'semver';

import * as api from '@/api/application.ts';

import { Preview } from './preview.tsx';
import { Updating } from './updating.tsx';

type UpdateProps = {
  setIsLocked: (isClosable: boolean) => void;
};

type Status = '' | 'loading' | 'updating' | 'outdated' | 'latest' | 'failed';

const maxReleaseBytes = 256 * 1024 * 1024;

export const Update = ({ setIsLocked }: UpdateProps) => {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);

  const [status, setStatus] = useState<Status>('');
  const [currentVersion, setCurrentVersion] = useState('');
  const [latestVersion, setLatestVersion] = useState('');
  const [errMsg, setErrMsg] = useState('');
  const [tipMsg, setTipMsg] = useState('');
  const [releaseFile, setReleaseFile] = useState<File | null>(null);
  const [uploadProgress, setUploadProgress] = useState<number | undefined>(undefined);
  const [fileError, setFileError] = useState('');

  useEffect(() => {
    checkForUpdates();
  }, []);

  function checkForUpdates() {
    if (status === 'loading' || status === 'updating') return;
    setStatus('loading');

    api
      .getVersion()
      .then((rsp: any) => {
        if (rsp.code !== 0 || !rsp.data) {
          setStatus('failed');
          setErrMsg(t('settings.update.queryFailed'));
          return;
        }

        setCurrentVersion(rsp.data.current);
        setLatestVersion(rsp.data.latest);

        const isLatest = semver.gt(rsp.data.latest, rsp.data.current);
        if (isLatest) {
          setTipMsg(t('settings.update.available'));
        }
        setStatus(!isLatest ? 'latest' : 'outdated');
      })
      .catch(() => {
        setStatus('failed');
        setErrMsg(t('settings.update.queryFailed'));
      });
  }

  function update() {
    if (status !== 'outdated') return;

    setIsLocked(true);
    setUploadProgress(undefined);
    setStatus('updating');

    api.update().then((rsp: any) => {
      if (rsp.code !== 0) {
        setIsLocked(false);
        setStatus('failed');
        setErrMsg(t('settings.update.updateFailed'));
      }
    });
  }

  function selectFile() {
    if (inputRef.current) {
      inputRef.current.value = '';
    }
    inputRef.current?.click();
  }

  function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) {
      return;
    }

    const name = file.name.toLowerCase();
    if ((!name.endsWith('.tar.gz') && !name.endsWith('.tgz')) || file.size > maxReleaseBytes) {
      setReleaseFile(null);
      setFileError(t('settings.update.invalidFile'));
      return;
    }

    setFileError('');
    setReleaseFile(file);
  }

  function installRelease() {
    if (!releaseFile || status === 'updating') {
      return;
    }

    setIsLocked(true);
    setUploadProgress(0);
    setStatus('updating');

    const formData = new FormData();
    formData.append('file', releaseFile);

    api
      .uploadRelease(formData, (progress) => {
        setUploadProgress(progress);
      })
      .then((rsp) => {
        if (rsp.code !== 0) {
          setIsLocked(false);
          setStatus('failed');
          setErrMsg(rsp.msg || t('settings.update.updateFailed'));
          return;
        }

        window.setTimeout(() => {
          window.location.reload();
        }, 20 * 1000);
      })
      .catch(() => {
        setIsLocked(false);
        setStatus('failed');
        setErrMsg(t('settings.update.updateFailed'));
      });
  }

  return (
    <>
      <div className="text-base font-bold">{t('settings.update.title')}</div>
      <Divider className="opacity-50" />

      <Preview checkForUpdates={checkForUpdates} />
      <Divider className="opacity-50" />

      <div className="flex min-h-[400px] flex-col justify-between">
        {status === 'loading' && (
          <div className="flex justify-center pt-24">
            <Spin indicator={<LoadingOutlined spin />} size="large" />
          </div>
        )}

        {status === 'updating' && <Updating uploadProgress={uploadProgress} />}

        {status === 'latest' && (
          <Result
            status="success"
            icon={<SmileOutlined />}
            title={currentVersion}
            subTitle={t('settings.update.isLatest')}
            extra={[
              <Button key="confirm" onClick={checkForUpdates}>
                {t('settings.update.title')}
              </Button>
            ]}
          />
        )}

        {status === 'outdated' && (
          <Result
            status="warning"
            icon={<RocketOutlined />}
            title={`${currentVersion} -> ${latestVersion}`}
            subTitle={tipMsg}
            extra={[
              <Button key="confirm" type="primary" onClick={update}>
                {t('settings.update.confirm')}
              </Button>
            ]}
          />
        )}

        {status === 'failed' && (
          <Result
            status="error"
            subTitle={errMsg}
            extra={[
              <Button key="retry" onClick={checkForUpdates}>
                {t('settings.update.title')}
              </Button>
            ]}
          />
        )}

        {status !== 'loading' && status !== 'updating' && (
          <>
            <Divider className="opacity-50" />
            <div className="flex flex-col items-center space-y-3 pb-6">
              <span className="text-center text-sm text-neutral-400">
                {t('settings.update.uploadDesc')}
              </span>
              <input
                ref={inputRef}
                type="file"
                accept=".gz,.tgz,application/gzip"
                className="hidden"
                onChange={onFileChange}
              />
              <div className="flex flex-wrap justify-center gap-2">
                <Button icon={<UploadOutlined />} onClick={selectFile}>
                  {releaseFile ? releaseFile.name : t('settings.update.chooseFile')}
                </Button>
                <Button type="primary" disabled={!releaseFile} onClick={installRelease}>
                  {t('settings.update.install')}
                </Button>
              </div>
              {fileError && <span className="text-sm text-red-400">{fileError}</span>}
            </div>
          </>
        )}

        <div className="flex justify-center space-x-4">
          <Button
            type="link"
            size="small"
            href="https://github.com/sipeed/NanoKVM-Pro/blob/main/CHANGELOG.md"
            target="_blank"
          >
            {t('settings.update.changelog')}
          </Button>
          <Button
            type="link"
            size="small"
            href="https://github.com/pekishev/NanoKVM-Pro/releases"
            target="_blank"
          >
            {t('settings.update.forkReleases')}
          </Button>
        </div>
      </div>
    </>
  );
};
