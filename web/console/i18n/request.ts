import { getRequestConfig } from 'next-intl/server';

async function loadNamespace(locale: string, ns: string) {
  try {
    return (await import(`../messages/${locale}/${ns}.json`)).default;
  } catch {
    return (await import(`../messages/en/${ns}.json`)).default;
  }
}

const namespaces = [
  'common',
  'nav',
  'dashboard',
  'devices',
  'devices-proxmox',
  'devices-docker',
  'devices-truenas',
  'terminal',
  'desktop',
  'services',
  'logs',
  'alerts',
  'incidents',
  'settings',
  'actions',
  'files',
  'topology',
  'auth',
  'reliability',
  'telemetry',
  'groups',
  'recordings',
  'users',
  'security',
  'notifications',
  'webhooks',
  'schedules',
  'notification-center',
  'audit-log',
  'api-docs',
] as const;

export default getRequestConfig(async ({ requestLocale }) => {
  const locale = (await requestLocale) || 'en';

  const entries = await Promise.all(
    namespaces.map(async (ns) => [ns, await loadNamespace(locale, ns)] as const)
  );

  return {
    locale,
    messages: Object.fromEntries(entries),
  };
});
