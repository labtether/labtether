import { expect, test, type Route } from "@playwright/test";

import {
  buildLiveStatusPayload,
  buildStatusPayload,
  installConsoleApiMocks,
  type MockRouteContext,
} from "./helpers/consoleApiMocks";

const keyDocker = "services.discovery_default_docker_enabled";
const keyProxy = "services.discovery_default_proxy_enabled";
const keyPortScan = "services.discovery_default_port_scan_enabled";
const keyLANScan = "services.discovery_default_lan_scan_enabled";

const runtimeDefaults: Record<string, string> = {
  [keyDocker]: "true",
  [keyProxy]: "true",
  "services.discovery_default_proxy_traefik_enabled": "true",
  "services.discovery_default_proxy_caddy_enabled": "true",
  "services.discovery_default_proxy_npm_enabled": "true",
  [keyPortScan]: "true",
  "services.discovery_default_port_scan_include_listening": "true",
  "services.discovery_default_port_scan_ports": "",
  [keyLANScan]: "false",
  "services.discovery_default_lan_scan_cidrs": "",
  "services.discovery_default_lan_scan_ports": "",
  "services.discovery_default_lan_scan_max_hosts": "64",
};

type MockService = {
  id: string;
  name: string;
  source: "docker" | "proxy" | "scan";
  url: string;
  host_asset_id: string;
  category: string;
  status: string;
  icon_key: string;
  metadata?: Record<string, string>;
};

const allServices: MockService[] = [
  {
    id: "svc-docker",
    name: "Docker App",
    source: "docker",
    url: "http://host-1:3000",
    host_asset_id: "host-1",
    category: "Development",
    status: "up",
    icon_key: "globe",
  },
  {
    id: "svc-proxy",
    name: "Traefik Route",
    source: "proxy",
    url: "https://route.home.arpa",
    host_asset_id: "host-1",
    category: "Networking",
    status: "up",
    icon_key: "traefik",
    metadata: {
      proxy_provider: "traefik",
    },
  },
  {
    id: "svc-local",
    name: "Local Scan UI",
    source: "scan",
    url: "http://host-1:8080",
    host_asset_id: "host-1",
    category: "Management",
    status: "up",
    icon_key: "globe",
    metadata: {
      scan_scope: "local",
      scan_target_host: "host-1",
    },
  },
  {
    id: "svc-lan",
    name: "LAN Scan UI",
    source: "scan",
    url: "http://192.168.10.44:8080",
    host_asset_id: "host-1",
    category: "Management",
    status: "up",
    icon_key: "globe",
    metadata: {
      scan_scope: "lan",
      scan_target_host: "192.168.10.44",
    },
  },
];

function buildRuntimeSettingsPayload(overrides: Record<string, string>) {
  const entries = [
    {
      key: keyDocker,
      label: "Default Agent Docker Discovery",
      description: "Default agent setting for Docker-backed service discovery.",
      type: "bool",
      default_value: runtimeDefaults[keyDocker],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_DOCKER_ENABLED",
    },
    {
      key: keyProxy,
      label: "Default Agent Proxy API Discovery",
      description: "Default agent setting for reverse-proxy API discovery.",
      type: "bool",
      default_value: runtimeDefaults[keyProxy],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_ENABLED",
    },
    {
      key: "services.discovery_default_proxy_traefik_enabled",
      label: "Default Agent Traefik API Discovery",
      description: "Default agent setting for Traefik API route discovery.",
      type: "bool",
      default_value: runtimeDefaults["services.discovery_default_proxy_traefik_enabled"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_TRAEFIK_ENABLED",
    },
    {
      key: "services.discovery_default_proxy_caddy_enabled",
      label: "Default Agent Caddy API Discovery",
      description: "Default agent setting for Caddy admin API route discovery.",
      type: "bool",
      default_value: runtimeDefaults["services.discovery_default_proxy_caddy_enabled"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_CADDY_ENABLED",
    },
    {
      key: "services.discovery_default_proxy_npm_enabled",
      label: "Default Agent Nginx Proxy Manager Discovery",
      description: "Default agent setting for Nginx Proxy Manager API route discovery.",
      type: "bool",
      default_value: runtimeDefaults["services.discovery_default_proxy_npm_enabled"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PROXY_NPM_ENABLED",
    },
    {
      key: keyPortScan,
      label: "Default Agent Local Port Scan",
      description: "Default agent setting for local host port scanning.",
      type: "bool",
      default_value: runtimeDefaults[keyPortScan],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_ENABLED",
    },
    {
      key: "services.discovery_default_port_scan_include_listening",
      label: "Default Agent Scan Listening Ports",
      description: "Default agent setting for including listening sockets in local port scans.",
      type: "bool",
      default_value: runtimeDefaults["services.discovery_default_port_scan_include_listening"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_INCLUDE_LISTENING",
    },
    {
      key: "services.discovery_default_port_scan_ports",
      label: "Default Agent Local Scan Ports",
      description: "Optional default local scan port list for agents.",
      type: "string",
      default_value: runtimeDefaults["services.discovery_default_port_scan_ports"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_PORT_SCAN_PORTS",
    },
    {
      key: keyLANScan,
      label: "Default Agent LAN Scan",
      description: "Default agent setting for LAN CIDR service scanning.",
      type: "bool",
      default_value: runtimeDefaults[keyLANScan],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_ENABLED",
    },
    {
      key: "services.discovery_default_lan_scan_cidrs",
      label: "Default Agent LAN CIDRs",
      description: "Optional default CIDR list for agent LAN scanning.",
      type: "string",
      default_value: runtimeDefaults["services.discovery_default_lan_scan_cidrs"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_CIDRS",
    },
    {
      key: "services.discovery_default_lan_scan_ports",
      label: "Default Agent LAN Scan Ports",
      description: "Optional default port list for agent LAN scanning.",
      type: "string",
      default_value: runtimeDefaults["services.discovery_default_lan_scan_ports"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_PORTS",
    },
    {
      key: "services.discovery_default_lan_scan_max_hosts",
      label: "Default Agent LAN Scan Host Cap",
      description: "Default maximum LAN hosts probed per agent scan cycle.",
      type: "int",
      default_value: runtimeDefaults["services.discovery_default_lan_scan_max_hosts"],
      env_var: "LABTETHER_SERVICES_DISCOVERY_DEFAULT_LAN_SCAN_MAX_HOSTS",
    },
  ].map((entry) => {
    const override = overrides[entry.key];
    const hasOverride = typeof override === "string" && override.trim() !== "";
    const effectiveValue = hasOverride ? override : entry.default_value;
    return {
      ...entry,
      scope: "services",
      env_value: entry.default_value,
      override_value: hasOverride ? override : undefined,
      effective_value: effectiveValue,
      source: hasOverride ? "ui" : "default",
    };
  });

  return {
    settings: entries,
    overrides,
  };
}

function boolValue(overrides: Record<string, string>, key: string): boolean {
  const raw = (overrides[key] ?? runtimeDefaults[key] ?? "").trim().toLowerCase();
  return raw === "true";
}

function filteredServices(overrides: Record<string, string>): MockService[] {
  const dockerEnabled = boolValue(overrides, keyDocker);
  const proxyEnabled = boolValue(overrides, keyProxy);
  const localScanEnabled = boolValue(overrides, keyPortScan);
  const lanScanEnabled = boolValue(overrides, keyLANScan);

  return allServices.filter((service) => {
    if (service.source === "docker") {
      return dockerEnabled;
    }
    if (service.source === "proxy") {
      return proxyEnabled;
    }
    if (service.source === "scan") {
      const scope = service.metadata?.scan_scope === "lan" ? "lan" : "local";
      return scope === "lan" ? lanScanEnabled : localScanEnabled;
    }
    return true;
  });
}

async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

test("services discovery defaults toggle source include/exclude behavior", async ({ page }) => {
  const unmocked = new Set<string>();
  let runtimeOverrides: Record<string, string> = {};

  function customRoute(context: MockRouteContext): Promise<boolean> | boolean {
    const { method, pathname, requestBody, route } = context;

    if (pathname === "/api/settings/runtime") {
      if (method === "PATCH") {
        const values = (requestBody.values as Record<string, string> | undefined) ?? {};
        runtimeOverrides = {
          ...runtimeOverrides,
          ...values,
        };
      }
      return fulfillJSON(route, buildRuntimeSettingsPayload(runtimeOverrides)).then(() => true);
    }

    if (pathname === "/api/settings/runtime/reset") {
      const keys = (requestBody.keys as string[] | undefined) ?? [];
      for (const key of keys) {
        delete runtimeOverrides[key];
      }
      return fulfillJSON(route, buildRuntimeSettingsPayload(runtimeOverrides)).then(() => true);
    }

    if (pathname === "/api/services/web") {
      const services = filteredServices(runtimeOverrides);
      const dockerCount = services.filter((item) => item.source === "docker").length;
      const proxyCount = services.filter((item) => item.source === "proxy").length;
      const localScanCount = services.filter((item) => item.source === "scan" && item.metadata?.scan_scope !== "lan").length;
      const lanScanCount = services.filter((item) => item.source === "scan" && item.metadata?.scan_scope === "lan").length;
      const payload = {
        services,
        discovery_stats: [
          {
            host_asset_id: "host-1",
            last_seen: new Date().toISOString(),
            discovery: {
              collected_at: new Date().toISOString(),
              cycle_duration_ms: 120,
              total_services: services.length,
              sources: {
                docker: {
                  enabled: boolValue(runtimeOverrides, keyDocker),
                  duration_ms: 20,
                  services_found: dockerCount,
                },
                proxy: {
                  enabled: boolValue(runtimeOverrides, keyProxy),
                  duration_ms: 25,
                  services_found: proxyCount,
                },
                local_scan: {
                  enabled: boolValue(runtimeOverrides, keyPortScan),
                  duration_ms: 30,
                  services_found: localScanCount,
                },
                lan_scan: {
                  enabled: boolValue(runtimeOverrides, keyLANScan),
                  duration_ms: 45,
                  services_found: lanScanCount,
                },
              },
              final_source_count: {
                docker: dockerCount,
                proxy: proxyCount,
                scan: localScanCount + lanScanCount,
              },
            },
          },
        ],
      };
      return fulfillJSON(route, payload).then(() => true);
    }

    if (pathname === "/api/services/web/sync") {
      return fulfillJSON(route, {
        requested: ["host-1"],
        queued: 1,
        sent_to: ["host-1"],
      }).then(() => true);
    }

    if (pathname === "/api/services/web/icon-library") {
      return fulfillJSON(route, { icons: [] }).then(() => true);
    }

    if (pathname === "/api/services/web/compat") {
      return fulfillJSON(route, { compatible: [] }).then(() => true);
    }

    if (pathname === "/api/v1/tls/info") {
      return fulfillJSON(route, {
        tls_enabled: false,
        cert_type: "none",
        ca_available: false,
      }).then(() => true);
    }

    return false;
  }

  await installConsoleApiMocks(page, {
    statusPayload: buildStatusPayload({
      assets: [
        {
          id: "host-1",
          name: "Node Alpha",
          type: "node",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: new Date().toISOString(),
          metadata: {},
        },
      ],
    }),
    liveStatusPayload: buildLiveStatusPayload({
      assets: [
        {
          id: "host-1",
          name: "Node Alpha",
          type: "node",
          source: "agent",
          status: "online",
          platform: "linux",
          last_seen_at: new Date().toISOString(),
          metadata: {},
        },
      ],
    }),
    customRoute,
    unmocked,
  });

  await page.goto("/services");
  await expect(page.getByRole("heading", { name: "Services", level: 1, exact: true })).toBeVisible();

  await expect(page.getByText("Docker App")).toBeVisible();
  await expect(page.getByText("Traefik Route")).toBeVisible();
  await expect(page.getByText("Local Scan UI")).toBeVisible();
  await expect(page.getByText("LAN Scan UI")).toHaveCount(0);

  await page.goto("/settings");
  await expect(page.getByRole("heading", { name: "Settings", level: 1, exact: true })).toBeVisible();

  await page.getByTestId(`service-discovery-default-${keyDocker}`).selectOption("false");
  await page.getByTestId(`service-discovery-default-${keyProxy}`).selectOption("false");
  await page.getByTestId(`service-discovery-default-${keyPortScan}`).selectOption("false");
  await page.getByTestId(`service-discovery-default-${keyLANScan}`).selectOption("true");
  await page.getByRole("button", { name: "Save Discovery Defaults", exact: true }).click();
  await expect(page.locator("text=Runtime settings saved.").first()).toBeVisible();

  await page.goto("/services");
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText("Docker App")).toHaveCount(0);
  await expect(page.getByText("Traefik Route")).toHaveCount(0);
  await expect(page.getByText("Local Scan UI")).toHaveCount(0);
  await expect(page.getByText("LAN Scan UI")).toBeVisible();

  await page.goto("/settings");
  await page.getByTestId(`service-discovery-default-${keyDocker}`).selectOption("true");
  await page.getByRole("button", { name: "Save Discovery Defaults", exact: true }).click();
  await expect(page.locator("text=Runtime settings saved.").first()).toBeVisible();

  await page.goto("/services");
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText("Docker App")).toBeVisible();
  await expect(page.getByText("Traefik Route")).toHaveCount(0);
  await expect(page.getByText("Local Scan UI")).toHaveCount(0);
  await expect(page.getByText("LAN Scan UI")).toBeVisible();

  await page.goto("/settings");
  await page.getByTestId(`service-discovery-default-${keyProxy}`).selectOption("true");
  await page.getByRole("button", { name: "Save Discovery Defaults", exact: true }).click();
  await expect(page.locator("text=Runtime settings saved.").first()).toBeVisible();

  await page.goto("/services");
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText("Docker App")).toBeVisible();
  await expect(page.getByText("Traefik Route")).toBeVisible();
  await expect(page.getByText("Local Scan UI")).toHaveCount(0);
  await expect(page.getByText("LAN Scan UI")).toBeVisible();

  await page.goto("/settings");
  await page.getByTestId(`service-discovery-default-${keyPortScan}`).selectOption("true");
  await page.getByRole("button", { name: "Save Discovery Defaults", exact: true }).click();
  await expect(page.locator("text=Runtime settings saved.").first()).toBeVisible();

  await page.goto("/services");
  await page.getByRole("button", { name: "Refresh", exact: true }).click();
  await expect(page.getByText("Docker App")).toBeVisible();
  await expect(page.getByText("Traefik Route")).toBeVisible();
  await expect(page.getByText("Local Scan UI")).toBeVisible();
  await expect(page.getByText("LAN Scan UI")).toBeVisible();

  expect([...unmocked]).toEqual([]);
});

test("services page tolerates malformed discovery collections and null LabTether metadata", async ({ page }) => {
  const statusPayload = buildStatusPayload({
    assets: [
      {
        id: "host-1",
        name: "Node Alpha",
        type: "node",
        source: "agent",
        status: "online",
        platform: "linux",
        last_seen_at: new Date().toISOString(),
        metadata: {},
      },
    ],
  });

  await installConsoleApiMocks(page, {
    statusPayload,
    liveStatusPayload: buildLiveStatusPayload({
      assets: statusPayload.assets as unknown[],
    }),
    customRoute: async ({ pathname, method, fulfillJSON }) => {
      if (pathname === "/api/services/web" && method === "GET") {
        await fulfillJSON({
          services: [
            {
              id: "svc-bootstrap",
              service_key: "labtether",
              name: "LabTether API",
              category: "Management",
              url: "http://host-1:8080",
              source: "docker",
              status: "up",
              response_ms: 24,
              host_asset_id: "host-1",
              icon_key: "globe",
              metadata: null,
            },
            {
              id: "svc-grafana",
              service_key: "grafana",
              name: "Grafana",
              category: "Monitoring",
              url: "http://host-1:3000",
              source: "docker",
              status: "up",
              response_ms: 42,
              host_asset_id: "host-1",
              icon_key: "grafana",
            },
          ],
          discovery_stats: null,
          suggestions: null,
        }, 200);
        return true;
      }

      if (pathname === "/api/services/web/icon-library" && method === "GET") {
        await fulfillJSON({ icons: null }, 200);
        return true;
      }

      if (pathname === "/api/services/web/compat" && method === "GET") {
        await fulfillJSON({ compatible: [] }, 200);
        return true;
      }

      return false;
    },
  });

  await page.goto("/services");

  await expect(page.getByRole("heading", { name: "Services", level: 1, exact: true })).toBeVisible();
  await expect(page.locator('[data-service-name="Grafana"]')).toHaveCount(1);
  await expect(page.getByText("Cannot read properties")).toHaveCount(0);
});
