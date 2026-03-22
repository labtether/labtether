import { createNavigation } from 'next-intl/navigation';

export const locales = ['en', 'de', 'fr', 'es', 'zh'] as const;

export const defaultLocale = 'en';

export const { Link, useRouter, usePathname, redirect } = createNavigation({
  locales,
  defaultLocale,
  localePrefix: 'always',
});
