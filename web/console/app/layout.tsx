import "@xterm/xterm/css/xterm.css";
import "./globals.css";
import type { Metadata } from "next";
import { getLocale } from "next-intl/server";
import { Inter, Sora, JetBrains_Mono } from "next/font/google";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

const sora = Sora({
  subsets: ["latin"],
  variable: "--font-sora",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-jetbrains-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "LabTether Console",
  description: "Homelab operations console",
  icons: {
    icon: "/logo.svg",
  },
};

/**
 * Blocking script that reads theme preferences from localStorage and applies
 * them to <body> before first paint.  This ensures login, setup, and console
 * pages all render with the user's chosen theme/accent — even pages outside
 * the (console) route group where ThemeProvider doesn't mount.
 */
const THEME_INIT_SCRIPT = [
  '(function(){try{',
  'var d=document.body.dataset,s=localStorage,',
  't=s.getItem("labtether.theme"),',
  'a=s.getItem("lt-accent"),',
  'n=s.getItem("labtether.density");',
  'if(t){d.theme=t}',
  'else{d.theme=window.matchMedia("(prefers-color-scheme:light)").matches?"light":"oled"}',
  'if(a&&a!=="neon-rose"){d.accent=a}',
  'if(n){d.density=n}',
  '}catch(e){}})()',
].join('');

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale();
  return (
    <html lang={locale} className={`${inter.variable} ${sora.variable} ${jetbrainsMono.variable}`}>
      {/* suppressHydrationWarning: blocking script sets data-* attrs before React hydrates */}
      <body suppressHydrationWarning>
        <script
          // Static constant — no user input, safe from XSS
          dangerouslySetInnerHTML={{ __html: THEME_INIT_SCRIPT }}
        />
        {children}
      </body>
    </html>
  );
}
