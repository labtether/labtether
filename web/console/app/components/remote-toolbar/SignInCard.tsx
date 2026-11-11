"use client";

import { useTranslations } from "next-intl";

/**
 * The remote screen asks for a login: a card over the screen with a username (when the screen asks for one) and a
 * password. onCancel is optional; without it the card has only Sign in. macName is the device's name when it is a
 * Mac: its Screen Sharing asks for an account on that Mac, so the card says so.
 */
export function SignInCard({
  needsUsername,
  username,
  onUsernameChange,
  password,
  onPasswordChange,
  onSubmit,
  onCancel,
  macName,
}: {
  needsUsername: boolean;
  username: string;
  onUsernameChange: (value: string) => void;
  password: string;
  onPasswordChange: (value: string) => void;
  onSubmit: () => void;
  onCancel?: () => void;
  macName?: string;
}) {
  const t = useTranslations("desktop.signIn");
  const macTitle = needsUsername && macName ? t("macTitle", { name: macName }) : null;
  return (
    <div className="rv-over">
      <form
        className="rv-card"
        role="dialog"
        aria-modal="true"
        aria-labelledby="rv-signin-title"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit();
        }}
      >
        <h2 id="rv-signin-title">{macTitle ?? t(needsUsername ? "title" : "vncTitle")}</h2>
        {macTitle ? <p className="rv-card-sub">{t("macAccount")}</p> : null}
        {needsUsername ? (
          <div className="rv-field">
            <label className="rv-field-l" htmlFor="rv-signin-user">
              {t("username")}
            </label>
            <input
              id="rv-signin-user"
              className="rv-input"
              value={username}
              onChange={(event) => onUsernameChange(event.target.value)}
              autoComplete="username"
              spellCheck={false}
              autoFocus
            />
          </div>
        ) : null}
        <div className="rv-field">
          <label className="rv-field-l" htmlFor="rv-signin-pass">
            {t("password")}
          </label>
          <input
            id="rv-signin-pass"
            className="rv-input"
            type="password"
            value={password}
            onChange={(event) => onPasswordChange(event.target.value)}
            autoComplete="current-password"
            autoFocus={!needsUsername}
          />
        </div>
        <div className="rv-card-foot">
          {onCancel ? (
            <button type="button" className="rv-btn" onClick={onCancel}>
              {t("cancel")}
            </button>
          ) : null}
          <button type="submit" className="rv-btn primary">
            {t("submit")}
          </button>
        </div>
      </form>
    </div>
  );
}
