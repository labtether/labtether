import { expect, it } from "vitest";
import deDesktop from "../../../../messages/de/desktop.json";
import deTerminal from "../../../../messages/de/terminal.json";
import enDesktop from "../../../../messages/en/desktop.json";
import enTerminal from "../../../../messages/en/terminal.json";
import esDesktop from "../../../../messages/es/desktop.json";
import esTerminal from "../../../../messages/es/terminal.json";
import frDesktop from "../../../../messages/fr/desktop.json";
import frTerminal from "../../../../messages/fr/terminal.json";
import zhDesktop from "../../../../messages/zh/desktop.json";
import zhTerminal from "../../../../messages/zh/terminal.json";

// Risk: while Remote View or a terminal connects, the step line showed "Getting stream ticket", an internal word a
// person cannot act on. The steps say what the person waits for, in every language.
const steps: Record<string, string[]> = {
  en: [...Object.values(enDesktop.screen.step), ...Object.values(enTerminal.words)],
  de: [...Object.values(deDesktop.screen.step), ...Object.values(deTerminal.words)],
  es: [...Object.values(esDesktop.screen.step), ...Object.values(esTerminal.words)],
  fr: [...Object.values(frDesktop.screen.step), ...Object.values(frTerminal.words)],
  zh: [...Object.values(zhDesktop.screen.step), ...Object.values(zhTerminal.words)],
};

it.each(Object.keys(steps))("names no stream ticket in the %s connect steps", (locale) => {
  expect(steps[locale].filter((text) => /ticket|票据/i.test(text))).toEqual([]);
});
