import { formatMetadataLabel, formatMetadataValue } from "./formatters";

export type NodeMetadataSectionName =
  | "System"
  | "Hardware"
  | "CPU"
  | "Memory"
  | "Storage"
  | "Firmware"
  | "Network"
  | "Live telemetry"
  | "Additional";

export type NodeMetadataFieldSpec = {
  key: string;
  label: string;
  section: NodeMetadataSectionName;
};

export type NodeMetadataRow = {
  key: string;
  label: string;
  value: string;
};

export type NodeMetadataSection = {
  title: NodeMetadataSectionName;
  rows: NodeMetadataRow[];
};

export const nodeMetadataSectionOrder: NodeMetadataSectionName[] = [
  "System",
  "Hardware",
  "CPU",
  "Memory",
  "Storage",
  "Firmware",
  "Network",
  "Live telemetry",
  "Additional",
];

export const nodeMetadataFields: NodeMetadataFieldSpec[] = [
  { key: "hostname", label: "Hostname", section: "System" },
  { key: "agent", label: "Agent", section: "System" },
  { key: "agent_device_fingerprint", label: "Device Fingerprint", section: "System" },
  { key: "agent_device_key_alg", label: "Device Key Algorithm", section: "System" },
  { key: "agent_identity_verified_at", label: "Identity Verified At", section: "System" },
  { key: "service_backend", label: "Service Backend", section: "System" },
  { key: "package_backend", label: "Package Backend", section: "System" },
  { key: "log_backend", label: "Log Backend", section: "System" },
  { key: "endpoint_id", label: "Endpoint ID", section: "System" },
  { key: "url", label: "Endpoint URL", section: "System" },
  { key: "portainer_endpoint_name", label: "Portainer Endpoint Name", section: "System" },
  { key: "portainer_version", label: "Portainer Version", section: "System" },
  { key: "portainer_database_version", label: "Portainer Database", section: "System" },
  { key: "portainer_build_number", label: "Portainer Build", section: "System" },
  { key: "cap_services", label: "Service Capabilities", section: "System" },
  { key: "cap_packages", label: "Package Capabilities", section: "System" },
  { key: "cap_logs", label: "Log Capabilities", section: "System" },
  { key: "cap_schedules", label: "Schedule Capabilities", section: "System" },
  { key: "cap_network", label: "Network Capabilities", section: "System" },
  { key: "os_pretty_name", label: "OS", section: "System" },
  { key: "os_name", label: "OS Name", section: "System" },
  { key: "os_version_id", label: "OS Version ID", section: "System" },
  { key: "os_version", label: "OS Version", section: "System" },
  { key: "kernel_release", label: "Kernel Release", section: "System" },
  { key: "kernel_version", label: "Kernel Build", section: "System" },
  { key: "cpu_architecture", label: "Architecture", section: "System" },
  { key: "computer_vendor", label: "System Vendor", section: "Hardware" },
  { key: "computer_model", label: "System Model", section: "Hardware" },
  { key: "computer_version", label: "System Version", section: "Hardware" },
  {
    key: "motherboard_vendor",
    label: "Motherboard Vendor",
    section: "Hardware",
  },
  { key: "motherboard_model", label: "Motherboard Model", section: "Hardware" },
  {
    key: "motherboard_version",
    label: "Motherboard Version",
    section: "Hardware",
  },
  { key: "chassis_vendor", label: "Chassis Vendor", section: "Hardware" },
  { key: "chassis_type", label: "Chassis Type", section: "Hardware" },
  { key: "cpu_model", label: "CPU Model", section: "CPU" },
  { key: "cpu_vendor", label: "CPU Vendor", section: "CPU" },
  { key: "cpu_sockets", label: "Sockets", section: "CPU" },
  { key: "cpu_cores_physical", label: "Physical Cores", section: "CPU" },
  { key: "cpu_cores_per_socket", label: "Cores / Socket", section: "CPU" },
  { key: "cpu_threads_logical", label: "Logical Threads", section: "CPU" },
  { key: "cpu_threads_per_socket", label: "Threads / Socket", section: "CPU" },
  { key: "cpu_max_mhz", label: "Max Frequency", section: "CPU" },
  { key: "cpu_min_mhz", label: "Min Frequency", section: "CPU" },
  { key: "memory_total_bytes", label: "Installed Memory", section: "Memory" },
  { key: "image", label: "Image", section: "Storage" },
  { key: "ports", label: "Ports", section: "Storage" },
  { key: "stack", label: "Stack", section: "Storage" },
  { key: "container_id", label: "Container ID", section: "Storage" },
  { key: "stack_id", label: "Stack ID", section: "Storage" },
  { key: "entry_point", label: "Entry Point", section: "Storage" },
  { key: "created_by", label: "Created By", section: "Storage" },
  { key: "git_url", label: "Git URL", section: "Storage" },
  { key: "created_at", label: "Created At", section: "Storage" },
  { key: "portainer_container_count", label: "Containers", section: "Storage" },
  { key: "portainer_stack_count", label: "Stacks", section: "Storage" },
  { key: "portainer_stack_container_count", label: "Stack Containers", section: "Storage" },
  {
    key: "disk_root_total_bytes",
    label: "Root Disk Capacity",
    section: "Storage",
  },
  {
    key: "disk_root_available_bytes",
    label: "Root Disk Available",
    section: "Storage",
  },
  { key: "disk_percent", label: "Disk Used", section: "Storage" },
  { key: "bios_vendor", label: "BIOS Vendor", section: "Firmware" },
  { key: "bios_version", label: "BIOS Version", section: "Firmware" },
  { key: "bios_date", label: "BIOS Date", section: "Firmware" },
  { key: "network_interface_count", label: "Interfaces", section: "Network" },
  {
    key: "tailscale_installed",
    label: "Tailscale Installed",
    section: "Network",
  },
  {
    key: "tailscale_backend_state",
    label: "Tailscale State",
    section: "Network",
  },
  { key: "tailscale_tailnet", label: "Tailscale Tailnet", section: "Network" },
  {
    key: "tailscale_self_dns_name",
    label: "Tailscale DNS",
    section: "Network",
  },
  {
    key: "tailscale_self_tailscale_ip",
    label: "Tailscale IPs",
    section: "Network",
  },
  { key: "tailscale_exit_node", label: "Using Exit Node", section: "Network" },
  { key: "tailscale_version", label: "Tailscale Version", section: "Network" },
  { key: "network_backend", label: "Network Backend", section: "Network" },
  {
    key: "network_action_backend",
    label: "Network Action Backend",
    section: "Network",
  },
  {
    key: "network_rx_bytes_per_sec",
    label: "RX Throughput",
    section: "Network",
  },
  {
    key: "network_tx_bytes_per_sec",
    label: "TX Throughput",
    section: "Network",
  },
  { key: "cpu_percent", label: "CPU Utilization", section: "Live telemetry" },
  {
    key: "memory_percent",
    label: "Memory Utilization",
    section: "Live telemetry",
  },
  { key: "temp_celsius", label: "Temperature", section: "Live telemetry" },
  // Proxmox-specific fields
  { key: "proxmox_type", label: "Proxmox Type", section: "System" },
  { key: "node", label: "Proxmox Node", section: "System" },
  { key: "vmid", label: "Proxmox VMID", section: "System" },
  { key: "hastate", label: "HA State", section: "System" },
  { key: "template", label: "Template", section: "System" },
  { key: "last_backup_at", label: "Last Backup", section: "Storage" },
  { key: "days_since_backup", label: "Days Since Backup", section: "Storage" },
  { key: "backup_state", label: "Backup State", section: "Storage" },
  // PBS (Proxmox Backup Server) specific fields
  { key: "version", label: "Version", section: "System" },
  { key: "datastore_count", label: "Datastores", section: "Storage" },
  { key: "total_bytes", label: "Total Capacity", section: "Storage" },
  { key: "used_bytes", label: "Used Capacity", section: "Storage" },
  { key: "group_count", label: "Backup Groups", section: "Storage" },
  { key: "snapshot_count", label: "Snapshots", section: "Storage" },
  { key: "collector_endpoint_host", label: "Endpoint", section: "Network" },
];

export function buildNodeMetadataSections(
  metadata?: Record<string, string>,
): NodeMetadataSection[] {
  if (!metadata) {
    return [];
  }

  const entries = Object.entries(metadata).filter(
    ([, value]) => value.trim() !== "",
  );
  if (entries.length === 0) {
    return [];
  }

  const rowsBySection = new Map<NodeMetadataSectionName, NodeMetadataRow[]>();
  const consumedKeys = new Set<string>();

  for (const field of nodeMetadataFields) {
    const raw = metadata[field.key];
    if (typeof raw !== "string" || raw.trim() === "") {
      continue;
    }
    consumedKeys.add(field.key);

    const rows = rowsBySection.get(field.section) ?? [];
    rows.push({
      key: field.key,
      label: field.label,
      value: formatMetadataValue(field.key, raw),
    });
    rowsBySection.set(field.section, rows);
  }

  const extraRows = entries
    .filter(([key]) => !consumedKeys.has(key))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => ({
      key,
      label: formatMetadataLabel(key),
      value: formatMetadataValue(key, value),
    }));

  if (extraRows.length > 0) {
    rowsBySection.set("Additional", extraRows);
  }

  return nodeMetadataSectionOrder
    .map((section) => {
      const rows = rowsBySection.get(section);
      if (!rows || rows.length === 0) {
        return null;
      }
      return { title: section, rows };
    })
    .filter((section): section is NodeMetadataSection => section !== null);
}
