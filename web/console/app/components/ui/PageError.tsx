"use client";

import { Button } from "./Button";

type PageErrorProps = {
  title: string;
  message?: string;
  retryLabel?: string;
  onRetry?: () => void;
};

/** 2.0 error view for a page or section that failed to render: what happened, the error's own line, one button. */
export function PageError({ title, message, retryLabel, onRetry }: PageErrorProps) {
  return (
    <div className="kt-error" role="alert">
      <h2 className="kt-error-t">{title}</h2>
      {message ? <p className="kt-error-d">{message}</p> : null}
      {onRetry && retryLabel ? (
        <Button className="kt-error-a" onClick={onRetry}>
          {retryLabel}
        </Button>
      ) : null}
    </div>
  );
}
