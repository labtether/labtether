"use client";

import { useCallback, useMemo, useState } from "react";
import { Search, Server, HardDrive, Globe, FolderKey, Pencil, Monitor } from "lucide-react";
import { useFastStatus } from "../../../contexts/StatusContext";
import { useConnectedAgents } from "../../../hooks/useConnectedAgents";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";
import { useFileConnections } from "./useFileConnections";
import { ConnectionForm } from "./ConnectionForm";
import type { FileConnection } from "./fileConnectionsClient";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface NewTabPageProps {
  onOpenAgent: (assetId: string, name: string) => void;
  onOpenConnection: (connectionId: string, name: string, protocol: string) => void;
  onNewConnection: (protocol: string) => void;
}

// ---------------------------------------------------------------------------
// Protocol card metadata
// ---------------------------------------------------------------------------

type ProtocolMeta = {
  id: string;
  name: string;
  description: string;
  accent: string;
  accentBg: string;
  icon: typeof Server;
};

const PROTOCOLS: ProtocolMeta[] = [
  {
    id: "sftp",
    name: "SFTP",
    description: "Secure file transfer over SSH",
    accent: "text-blue-400",
    accentBg: "bg-blue-500/10 border-blue-500/20 hover:border-blue-500/40",
    icon: FolderKey,
  },
  {
    id: "smb",
    name: "SMB",
    description: "Windows/Samba file sharing",
    accent: "text-orange-400",
    accentBg: "bg-orange-500/10 border-orange-500/20 hover:border-orange-500/40",
    icon: HardDrive,
  },
  {
    id: "ftp",
    name: "FTP",
    description: "Classic file transfer protocol",
    accent: "text-teal-400",
    accentBg: "bg-teal-500/10 border-teal-500/20 hover:border-teal-500/40",
    icon: Server,
  },
  {
    id: "webdav",
    name: "WebDAV",
    description: "HTTP-based file access",
    accent: "text-cyan-400",
    accentBg: "bg-cyan-500/10 border-cyan-500/20 hover:border-cyan-500/40",
    icon: Globe,
  },
];

const PROTOCOL_DOT_COLOR: Record<string, string> = {
  sftp: "bg-blue-500",
  smb: "bg-orange-500",
  ftp: "bg-teal-500",
  webdav: "bg-cyan-500",
};

const PROTOCOL_MAP = Object.fromEntries(PROTOCOLS.map((p) => [p.id, p]));

const PROTOCOL_ICON_BG: Record<string, string> = {
  sftp: "bg-blue-500/10",
  smb: "bg-orange-500/10",
  ftp: "bg-teal-500/10",
  webdav: "bg-cyan-500/10",
};

const PROTOCOL_BADGE_STYLE: Record<string, string> = {
  sftp: "bg-blue-500/10 text-blue-400 border-blue-500/20",
  smb: "bg-orange-500/10 text-orange-400 border-orange-500/20",
  ftp: "bg-teal-500/10 text-teal-400 border-teal-500/20",
  webdav: "bg-cyan-500/10 text-cyan-400 border-cyan-500/20",
};

// ---------------------------------------------------------------------------
// Quick-connect URL parsing
// ---------------------------------------------------------------------------

function parseQuickConnectURL(input: string): { protocol: string; host: string; path: string } | null {
  const trimmed = input.trim();
  const match = trimmed.match(/^(sftp|smb|ftp|webdav):\/\/([^/]+)(\/.*)?$/i);
  if (!match) return null;
  return {
    protocol: match[1].toLowerCase(),
    host: match[2],
    path: match[3] ?? "/",
  };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function NewTabPage({ onOpenAgent, onOpenConnection, onNewConnection }: NewTabPageProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedProtocol, setSelectedProtocol] = useState<string | null>(null);
  const [editingConnection, setEditingConnection] = useState<FileConnection | null>(null);

  // Data sources
  const status = useFastStatus();
  const { connectedAgentIds } = useConnectedAgents();
  const { connections, loading: connectionsLoading, refetch } = useFileConnections();

  // Connected agents derived from status context
  const connectedAgents = useMemo(() => {
    const allAssets = status?.assets ?? [];
    return allAssets.filter((asset) => connectedAgentIds.has(asset.id));
  }, [status, connectedAgentIds]);

  // Filter agents and connections when searching
  const query = searchQuery.trim().toLowerCase();
  const isSearching = query.length > 0;

  const filteredAgents = useMemo(() => {
    if (!isSearching) return connectedAgents;
    return connectedAgents.filter((a) => a.name.toLowerCase().includes(query));
  }, [connectedAgents, isSearching, query]);

  const filteredConnections = useMemo(() => {
    if (!isSearching) return connections;
    return connections.filter(
      (c) =>
        c.name.toLowerCase().includes(query) ||
        c.protocol.toLowerCase().includes(query) ||
        c.host.toLowerCase().includes(query),
    );
  }, [connections, isSearching, query]);

  // ---------------------------------------------------------------------------
  // Quick-connect submit
  // ---------------------------------------------------------------------------

  const handleQuickConnect = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      const parsed = parseQuickConnectURL(searchQuery);
      if (parsed) {
        setSelectedProtocol(parsed.protocol);
      }
    },
    [searchQuery],
  );

  // ---------------------------------------------------------------------------
  // Connection form callback
  // ---------------------------------------------------------------------------

  const handleFormConnect = useCallback(
    (connection: FileConnection) => {
      setSelectedProtocol(null);
      setEditingConnection(null);
      void refetch();
      onOpenConnection(connection.id, connection.name, connection.protocol);
    },
    [onOpenConnection, refetch],
  );

  const handleFormCancel = useCallback(() => {
    setSelectedProtocol(null);
    setEditingConnection(null);
  }, []);

  const handleFormDeleted = useCallback(() => {
    setSelectedProtocol(null);
    setEditingConnection(null);
    void refetch();
  }, [refetch]);

  // ---------------------------------------------------------------------------
  // Render: Connection form inline mode
  // ---------------------------------------------------------------------------

  if (selectedProtocol) {
    return (
      <div className="flex-1 flex flex-col p-4 md:p-6 overflow-y-auto">
        <Card className="max-w-2xl mx-auto w-full">
          <ConnectionForm
            key={editingConnection?.id ?? "new"}
            protocol={selectedProtocol}
            existingConnection={editingConnection ?? undefined}
            onConnect={handleFormConnect}
            onCancel={handleFormCancel}
            onDeleted={handleFormDeleted}
          />
        </Card>
      </div>
    );
  }

  // ---------------------------------------------------------------------------
  // Render: Landing page
  // ---------------------------------------------------------------------------

  return (
    <div className="flex-1 flex flex-col gap-6 p-4 md:p-6 overflow-y-auto">
      {/* Quick-connect bar */}
      <form onSubmit={handleQuickConnect} className="max-w-2xl mx-auto w-full">
        <div className="relative">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--muted)] pointer-events-none" />
          <Input
            className="pl-9"
            placeholder="sftp://user@host, smb://server/share, or search saved..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
      </form>

      {/* Protocol cards */}
      {!isSearching && (
        <div className="max-w-2xl mx-auto w-full">
          <h3 className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider mb-3">
            New Connection
          </h3>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            {PROTOCOLS.map((proto) => {
              const Icon = proto.icon;
              return (
                <button
                  key={proto.id}
                  className={`flex flex-col items-center gap-2 p-4 rounded-lg border transition-all duration-[var(--dur-fast)] cursor-pointer bg-transparent select-none ${proto.accentBg}`}
                  onClick={() => {
                    setSelectedProtocol(proto.id);
                    onNewConnection(proto.id);
                  }}
                >
                  <Icon className={`w-6 h-6 ${proto.accent}`} />
                  <span className="text-sm font-medium text-[var(--text)]">{proto.name}</span>
                  <span className="text-[10px] text-[var(--muted)] text-center leading-tight">
                    {proto.description}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}

      {/* Connected agents */}
      {filteredAgents.length > 0 && (
        <div className="max-w-2xl mx-auto w-full">
          <h3 className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider mb-3">
            Connected Agents
          </h3>
          <div className="grid gap-2">
            {filteredAgents.map((agent) => (
              <button
                key={agent.id}
                className="group flex items-center gap-3 w-full rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-3 py-2.5
                  text-left transition-[border-color,box-shadow] duration-[var(--dur-fast)] cursor-pointer
                  hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)]
                  focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
                onClick={() => onOpenAgent(agent.id, agent.name)}
              >
                <span className="flex items-center justify-center h-7 w-7 rounded-md bg-green-500/10 shrink-0 relative">
                  <Monitor size={14} className="text-green-400" />
                  <span className="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-green-500" style={{ boxShadow: "0 0 6px 1px var(--ok-glow)" }} />
                </span>
                <div className="flex flex-col min-w-0 flex-1">
                  <span className="text-sm font-medium text-[var(--text)] truncate">{agent.name}</span>
                  {agent.platform && (
                    <span className="text-[10px] text-[var(--muted)] truncate">
                      {agent.platform}
                    </span>
                  )}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Saved connections */}
      {(filteredConnections.length > 0 || connectionsLoading) && (
        <div className="max-w-2xl mx-auto w-full">
          <h3 className="text-xs font-medium text-[var(--muted)] uppercase tracking-wider mb-3">
            Saved Connections
          </h3>
          {connectionsLoading ? (
            <div className="text-xs text-[var(--muted)] px-1">Loading...</div>
          ) : (
            <div className="grid gap-2">
              {filteredConnections.map((conn) => {
                const proto = PROTOCOL_MAP[conn.protocol];
                const ProtoIcon = proto?.icon ?? Server;
                return (
                  <div
                    key={conn.id}
                    role="button"
                    tabIndex={0}
                    className="group flex items-center gap-3 rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-3 py-2.5
                      transition-[border-color,box-shadow] duration-[var(--dur-fast)] cursor-pointer
                      hover:border-[var(--accent)]/40 hover:shadow-[0_0_12px_var(--accent-glow)]
                      focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--control-focus-ring)]"
                    onClick={() => onOpenConnection(conn.id, conn.name, conn.protocol)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        onOpenConnection(conn.id, conn.name, conn.protocol);
                      }
                    }}
                  >
                    <span
                      className={`flex items-center justify-center h-7 w-7 rounded-md shrink-0 ${
                        PROTOCOL_ICON_BG[conn.protocol] ?? "bg-[var(--surface)]"
                      }`}
                    >
                      <ProtoIcon
                        size={14}
                        className={`${proto?.accent ?? "text-[var(--muted)]"} transition-colors`}
                      />
                    </span>
                    <div className="flex flex-col min-w-0 flex-1">
                      <span className="text-sm font-medium text-[var(--text)] truncate">
                        {conn.name}
                      </span>
                      <span className="text-[10px] text-[var(--muted)] truncate font-mono">
                        {conn.host}{conn.port ? `:${conn.port}` : ""}
                      </span>
                    </div>
                    <span
                      className={`text-[10px] font-medium uppercase px-1.5 py-0.5 rounded border shrink-0 ${
                        PROTOCOL_BADGE_STYLE[conn.protocol] ?? "bg-[var(--surface)] text-[var(--muted)] border-[var(--line)]"
                      }`}
                    >
                      {conn.protocol}
                    </span>
                    <button
                      className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)]
                        opacity-0 group-hover:opacity-100 transition-[opacity,color,background-color] duration-[var(--dur-fast)]
                        cursor-pointer bg-transparent border-none shrink-0"
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditingConnection(conn);
                        setSelectedProtocol(conn.protocol);
                      }}
                      title="Edit connection"
                    >
                      <Pencil className="w-3.5 h-3.5" />
                    </button>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      )}

      {/* Empty state when searching yields no results */}
      {isSearching && filteredAgents.length === 0 && filteredConnections.length === 0 && (
        <div className="max-w-2xl mx-auto w-full text-center py-8">
          <p className="text-sm text-[var(--muted)]">
            No agents or connections matching &ldquo;{searchQuery}&rdquo;
          </p>
        </div>
      )}
    </div>
  );
}
