"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { useTranslations } from "next-intl";
import { Check, ChevronRight, Globe, LogOut, Moon, Sun } from "lucide-react";
import { useAuth } from "../../contexts/AuthContext";
import { useTheme } from "../../contexts/ThemeContext";
import { normalizeHubRole } from "../../lib/roles";
import { localeLabels, supportedLocales, useSwitchLocale } from "../LanguagePicker";

/** The sidebar's account button: who is signed in, and a menu with the theme, the language and Sign out. */
export function AccountMenu() {
  const t = useTranslations("nav");
  const { user, logout } = useAuth();
  const { theme, setTheme } = useTheme();
  const { locale, switchTo } = useSwitchLocale();
  if (!user) return null;

  const light = theme === "light";
  const name = user.username || "?";

  return (
    <Menu.Root>
      <Menu.Trigger className="lt-acct" aria-label={t("account")}>
        <span className="lt-av" aria-hidden="true">{name.slice(0, 1).toUpperCase()}</span>
        <span className="lt-who">
          {name}
          <span>{t(`roles.${normalizeHubRole(user.role)}`)}</span>
        </span>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" side="top" align="start" sideOffset={6}>
          <Menu.Item className="lt-menu-item" onSelect={() => setTheme(light ? "oled" : "light")}>
            {light ? <Moon aria-hidden="true" /> : <Sun aria-hidden="true" />}
            {t(light ? "darkTheme" : "lightTheme")}
          </Menu.Item>
          <Menu.Sub>
            <Menu.SubTrigger className="lt-menu-item">
              <Globe aria-hidden="true" />
              {t("language")}
              <span className="lt-menu-hint">{localeLabels[locale]}</span>
              <ChevronRight aria-hidden="true" />
            </Menu.SubTrigger>
            <Menu.Portal>
              <Menu.SubContent className="lt-menu" sideOffset={6}>
                <Menu.RadioGroup value={locale} onValueChange={switchTo}>
                  {supportedLocales.map((loc) => (
                    <Menu.RadioItem key={loc} value={loc} className="lt-menu-item">
                      {localeLabels[loc]}
                      <Menu.ItemIndicator className="lt-menu-check">
                        <Check aria-hidden="true" />
                      </Menu.ItemIndicator>
                    </Menu.RadioItem>
                  ))}
                </Menu.RadioGroup>
              </Menu.SubContent>
            </Menu.Portal>
          </Menu.Sub>
          <Menu.Separator className="lt-menu-sep" />
          <Menu.Item className="lt-menu-item" onSelect={() => void logout()}>
            <LogOut aria-hidden="true" />
            {t("signOut")}
          </Menu.Item>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}
