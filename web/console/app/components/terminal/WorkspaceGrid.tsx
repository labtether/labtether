"use client";

import { type ReactNode, useCallback } from "react";
import { Panel, Group, Separator } from "react-resizable-panels";
import type { PanelSizes } from "../../hooks/useWorkspaceTabs";

interface LayoutDef {
  count: number;
}

const layouts: Record<string, LayoutDef> = {
  single: { count: 1 },
  columns: { count: 2 },
  rows: { count: 2 },
  grid: { count: 4 },
  "main-side": { count: 2 },
  "main-bottom": { count: 2 },
};

const MIN_PANEL_SIZE = 20;

type Layout = { [id: string]: number };

/** Convert a v4 Layout map to a number[] using ordered panel IDs. */
function layoutToArray(layout: Layout, ids: string[]): number[] {
  return ids.map((id) => layout[id] ?? 50);
}

/** Convert a number[] to a v4 Layout map using ordered panel IDs. */
function arrayToLayout(sizes: number[], ids: string[]): Layout {
  const layout: Layout = {};
  ids.forEach((id, i) => {
    layout[id] = sizes[i] ?? 50;
  });
  return layout;
}

interface WorkspaceGridProps {
  layout: string;
  paneCount: number;
  panelSizes: PanelSizes;
  onPanelResize: (sizes: PanelSizes) => void;
  children: ReactNode[];
}

function ResizeHandle({ direction }: { direction: "horizontal" | "vertical" }) {
  return (
    <Separator
      className={`group relative flex items-center justify-center ${
        direction === "vertical"
          ? "w-[4px] cursor-col-resize"
          : "h-[4px] cursor-row-resize"
      }`}
    >
      <div
        className={`rounded-full bg-[var(--border)] transition-colors group-hover:bg-[var(--accent)] group-data-[state=dragging]:bg-[var(--accent)] ${
          direction === "vertical"
            ? "h-8 w-[2px]"
            : "h-[2px] w-8"
        }`}
      />
    </Separator>
  );
}

function PaneWrapper({ children }: { children: ReactNode }) {
  return (
    <div style={{ minHeight: 0, minWidth: 0, height: "100%", width: "100%", overflow: "hidden" }}>
      {children}
    </div>
  );
}

function singleAxisSizes(
  panelSizes: PanelSizes,
  defaults: number[],
): number[] {
  if (Array.isArray(panelSizes) && panelSizes.length === defaults.length) {
    return panelSizes;
  }
  return defaults;
}

function gridSizes(panelSizes: PanelSizes): {
  outer: number[];
  top: number[];
  bottom: number[];
} {
  if (
    panelSizes != null &&
    !Array.isArray(panelSizes) &&
    typeof panelSizes === "object" &&
    "outer" in panelSizes
  ) {
    return panelSizes;
  }
  return { outer: [50, 50], top: [50, 50], bottom: [50, 50] };
}

function SingleLayout({ children }: { children: ReactNode[] }) {
  return (
    <div style={{ display: "flex", flex: 1, minHeight: 0, padding: 2 }}>
      <PaneWrapper>{children[0]}</PaneWrapper>
    </div>
  );
}

const COL_IDS = ["left", "right"];

function ColumnsLayout({
  children,
  panelSizes,
  onPanelResize,
}: {
  children: ReactNode[];
  panelSizes: PanelSizes;
  onPanelResize: (sizes: PanelSizes) => void;
}) {
  const sizes = singleAxisSizes(panelSizes, [50, 50]);
  const handleResize = useCallback(
    (layout: Layout) => onPanelResize(layoutToArray(layout, COL_IDS)),
    [onPanelResize],
  );
  return (
    <Group orientation="horizontal" defaultLayout={arrayToLayout(sizes, COL_IDS)} onLayoutChanged={handleResize} style={{ flex: 1, minHeight: 0, padding: 2 }}>
      <Panel id="left" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[0]}</PaneWrapper>
      </Panel>
      <ResizeHandle direction="vertical" />
      <Panel id="right" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[1]}</PaneWrapper>
      </Panel>
    </Group>
  );
}

const ROW_IDS = ["top", "bottom"];

function RowsLayout({
  children,
  panelSizes,
  onPanelResize,
}: {
  children: ReactNode[];
  panelSizes: PanelSizes;
  onPanelResize: (sizes: PanelSizes) => void;
}) {
  const sizes = singleAxisSizes(panelSizes, [50, 50]);
  const handleResize = useCallback(
    (layout: Layout) => onPanelResize(layoutToArray(layout, ROW_IDS)),
    [onPanelResize],
  );
  return (
    <Group orientation="vertical" defaultLayout={arrayToLayout(sizes, ROW_IDS)} onLayoutChanged={handleResize} style={{ flex: 1, minHeight: 0, padding: 2 }}>
      <Panel id="top" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[0]}</PaneWrapper>
      </Panel>
      <ResizeHandle direction="horizontal" />
      <Panel id="bottom" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[1]}</PaneWrapper>
      </Panel>
    </Group>
  );
}

function MainSideLayout({
  children,
  panelSizes,
  onPanelResize,
}: {
  children: ReactNode[];
  panelSizes: PanelSizes;
  onPanelResize: (sizes: PanelSizes) => void;
}) {
  const sizes = singleAxisSizes(panelSizes, [66.6, 33.4]);
  const handleResize = useCallback(
    (layout: Layout) => onPanelResize(layoutToArray(layout, COL_IDS)),
    [onPanelResize],
  );
  return (
    <Group orientation="horizontal" defaultLayout={arrayToLayout(sizes, COL_IDS)} onLayoutChanged={handleResize} style={{ flex: 1, minHeight: 0, padding: 2 }}>
      <Panel id="left" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[0]}</PaneWrapper>
      </Panel>
      <ResizeHandle direction="vertical" />
      <Panel id="right" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[1]}</PaneWrapper>
      </Panel>
    </Group>
  );
}

function MainBottomLayout({
  children,
  panelSizes,
  onPanelResize,
}: {
  children: ReactNode[];
  panelSizes: PanelSizes;
  onPanelResize: (sizes: PanelSizes) => void;
}) {
  const sizes = singleAxisSizes(panelSizes, [66.6, 33.4]);
  const handleResize = useCallback(
    (layout: Layout) => onPanelResize(layoutToArray(layout, ROW_IDS)),
    [onPanelResize],
  );
  return (
    <Group orientation="vertical" defaultLayout={arrayToLayout(sizes, ROW_IDS)} onLayoutChanged={handleResize} style={{ flex: 1, minHeight: 0, padding: 2 }}>
      <Panel id="top" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[0]}</PaneWrapper>
      </Panel>
      <ResizeHandle direction="horizontal" />
      <Panel id="bottom" minSize={MIN_PANEL_SIZE}>
        <PaneWrapper>{children[1]}</PaneWrapper>
      </Panel>
    </Group>
  );
}

const GRID_OUTER_IDS = ["row-top", "row-bottom"];
const GRID_TOP_IDS = ["tl", "tr"];
const GRID_BOTTOM_IDS = ["bl", "br"];

function GridLayout({
  children,
  panelSizes,
  onPanelResize,
}: {
  children: ReactNode[];
  panelSizes: PanelSizes;
  onPanelResize: (sizes: PanelSizes) => void;
}) {
  const gs = gridSizes(panelSizes);

  const handleOuterResize = useCallback(
    (layout: Layout) => {
      const current = gridSizes(panelSizes);
      onPanelResize({ outer: layoutToArray(layout, GRID_OUTER_IDS), top: current.top, bottom: current.bottom });
    },
    [onPanelResize, panelSizes],
  );

  const handleTopResize = useCallback(
    (layout: Layout) => {
      const current = gridSizes(panelSizes);
      onPanelResize({ outer: current.outer, top: layoutToArray(layout, GRID_TOP_IDS), bottom: current.bottom });
    },
    [onPanelResize, panelSizes],
  );

  const handleBottomResize = useCallback(
    (layout: Layout) => {
      const current = gridSizes(panelSizes);
      onPanelResize({ outer: current.outer, top: current.top, bottom: layoutToArray(layout, GRID_BOTTOM_IDS) });
    },
    [onPanelResize, panelSizes],
  );

  return (
    <Group orientation="vertical" defaultLayout={arrayToLayout(gs.outer, GRID_OUTER_IDS)} onLayoutChanged={handleOuterResize} style={{ flex: 1, minHeight: 0, padding: 2 }}>
      <Panel id="row-top" minSize={MIN_PANEL_SIZE}>
        <Group orientation="horizontal" defaultLayout={arrayToLayout(gs.top, GRID_TOP_IDS)} onLayoutChanged={handleTopResize}>
          <Panel id="tl" minSize={MIN_PANEL_SIZE}>
            <PaneWrapper>{children[0]}</PaneWrapper>
          </Panel>
          <ResizeHandle direction="vertical" />
          <Panel id="tr" minSize={MIN_PANEL_SIZE}>
            <PaneWrapper>{children[1]}</PaneWrapper>
          </Panel>
        </Group>
      </Panel>
      <ResizeHandle direction="horizontal" />
      <Panel id="row-bottom" minSize={MIN_PANEL_SIZE}>
        <Group orientation="horizontal" defaultLayout={arrayToLayout(gs.bottom, GRID_BOTTOM_IDS)} onLayoutChanged={handleBottomResize}>
          <Panel id="bl" minSize={MIN_PANEL_SIZE}>
            <PaneWrapper>{children[2]}</PaneWrapper>
          </Panel>
          <ResizeHandle direction="vertical" />
          <Panel id="br" minSize={MIN_PANEL_SIZE}>
            <PaneWrapper>{children[3]}</PaneWrapper>
          </Panel>
        </Group>
      </Panel>
    </Group>
  );
}

export default function WorkspaceGrid({
  layout,
  paneCount,
  panelSizes,
  onPanelResize,
  children,
}: WorkspaceGridProps) {
  const def = layouts[layout] ?? layouts.single;
  const visibleCount = Math.min(paneCount, def.count);
  const visibleChildren = Array.isArray(children)
    ? children.slice(0, visibleCount)
    : [children];

  switch (layout) {
    case "columns":
      return (
        <ColumnsLayout panelSizes={panelSizes} onPanelResize={onPanelResize}>
          {visibleChildren}
        </ColumnsLayout>
      );
    case "rows":
      return (
        <RowsLayout panelSizes={panelSizes} onPanelResize={onPanelResize}>
          {visibleChildren}
        </RowsLayout>
      );
    case "grid":
      return (
        <GridLayout panelSizes={panelSizes} onPanelResize={onPanelResize}>
          {visibleChildren}
        </GridLayout>
      );
    case "main-side":
      return (
        <MainSideLayout panelSizes={panelSizes} onPanelResize={onPanelResize}>
          {visibleChildren}
        </MainSideLayout>
      );
    case "main-bottom":
      return (
        <MainBottomLayout panelSizes={panelSizes} onPanelResize={onPanelResize}>
          {visibleChildren}
        </MainBottomLayout>
      );
    default:
      return <SingleLayout>{visibleChildren}</SingleLayout>;
  }
}
