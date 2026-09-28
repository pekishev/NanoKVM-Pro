import semver from 'semver';

type VersionInfo = {
  source?: string;
  latest?: string;
  current?: string;
};

export function isUpdateAvailable(data?: VersionInfo): boolean {
  if (!data) {
    return false;
  }
  if (data.source === 'fork' || data.source === 'official') {
    return true;
  }
  if (typeof data.source === 'string') {
    return false;
  }
  if (!data.latest || !data.current || !semver.valid(data.latest) || !semver.valid(data.current)) {
    return false;
  }

  return semver.gt(data.latest, data.current);
}
