import type { Page } from "@playwright/test";
import { initDesktopBrowserEnvironment, type DesktopBrowserMockOptions } from "./desktopBrowserEnvironment";
import { initDesktopMediaMocks } from "./desktopMediaMocks";
import { initDesktopWebSocketMocks } from "./desktopWebSocketMocks";
import { initDesktopWebRTCMocks } from "./desktopWebRTCMocks";
import { initDesktopVNCMocks } from "./desktopVNCMocks";

export async function installDesktopBrowserMocks(
  page: Page,
  options: DesktopBrowserMockOptions = {},
) {
  // Playwright does not guarantee order across separate addInitScript calls.
  // Serialize self-contained owners together so all mocks share one environment
  // and install before application code starts, in a defined order.
  const initializers = [
    initDesktopMediaMocks,
    initDesktopWebSocketMocks,
    initDesktopWebRTCMocks,
    initDesktopVNCMocks,
  ];
  await page.addInitScript({
    content: `(() => {
      const environment = (${initDesktopBrowserEnvironment.toString()})(${JSON.stringify(options)});
      ${initializers.map(initialize => `(${initialize.toString()})(environment);`).join("\n")}
    })();`,
  });
}
