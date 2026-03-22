import { Code } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";
import type { TerminalSnippet } from "../../../hooks/useTerminalSnippets";

export function createSnippetsProvider(
  getSnippets: () => TerminalSnippet[],
  onInsert: (command: string) => void,
): PaletteProvider {
  return {
    id: "snippets",
    group: "Snippets",
    priority: 40,
    shortcut: "!",
    search(query: string): PaletteItem[] {
      // Strip leading "!" trigger character
      const stripped = query.startsWith("!") ? query.slice(1) : query;
      const q = stripped.trim().toLowerCase();

      return getSnippets()
        .filter((snippet) => {
          if (q === "") return true;
          return (
            snippet.name.toLowerCase().includes(q) ||
            snippet.description.toLowerCase().includes(q) ||
            snippet.command.toLowerCase().includes(q) ||
            snippet.scope.toLowerCase().includes(q)
          );
        })
        .map((snippet) => ({
          id: `snippet-${snippet.id}`,
          label: snippet.name,
          description: snippet.description || snippet.command,
          icon: Code,
          keywords: [snippet.scope, snippet.shortcut].filter(Boolean),
          action: () => onInsert(snippet.command),
        }));
    },
  };
}
