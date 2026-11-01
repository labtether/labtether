import { createContext, type ReactNode } from "react";

/**
 * The section a page belongs to (Automations, Settings), set by the console layout. PageHeader then shows the section's
 * name as the title with its tab row under it. No provider (tests, other layouts) means a plain header.
 */
export type PageSection = { title: string; tabs: ReactNode };
export const PageSectionContext = createContext<PageSection | null>(null);
