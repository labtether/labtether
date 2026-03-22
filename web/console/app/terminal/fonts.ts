export interface TerminalFontDef {
  id: string;
  name: string;
  family: string;
}

export const terminalFonts: TerminalFontDef[] = [
  { id: "jetbrains-mono", name: "JetBrains Mono", family: "'JetBrains Mono', monospace" },
  { id: "fira-code", name: "Fira Code", family: "'Fira Code', monospace" },
  { id: "source-code-pro", name: "Source Code Pro", family: "'Source Code Pro', monospace" },
  { id: "ibm-plex-mono", name: "IBM Plex Mono", family: "'IBM Plex Mono', monospace" },
  { id: "cascadia-code", name: "Cascadia Code", family: "'Cascadia Code', monospace" },
];

const defaultFont = terminalFonts[0];

export function getFontById(id: string): TerminalFontDef {
  return terminalFonts.find((f) => f.id === id) ?? defaultFont;
}
