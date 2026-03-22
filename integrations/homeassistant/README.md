# LabTether Home Assistant Integration

Custom Home Assistant integration that exposes LabTether-managed infrastructure as HA devices and entities.

Compatibility note: audited against Home Assistant Core `2026.3.1`, including config flow, options flow, reauthentication, and reconfiguration behavior.

## Features

- **Monitoring**: Each LabTether asset becomes an HA device with status and telemetry sensors
- **Control**: Start/stop VMs and containers via power switches
- **Automations**: Use the `labtether.run_action` service in HA automations
- **Dynamic discovery**: Newly discovered LabTether assets appear in Home Assistant without reloading the integration
- **Multi-hub safe**: Entity unique IDs and device identifiers are namespaced per LabTether connection
- **Upgrade-safe**: Existing older global entity unique IDs migrate forward to the per-connection scheme during setup
- **Bidirectional**: Works alongside LabTether's built-in HA connector without circular mirroring

## Installation

### Development (symlink)

```bash
ln -s /path/to/LabTether/integrations/homeassistant/custom_components/labtether \
      /path/to/ha-config/custom_components/labtether
```

### Manual

Copy the `custom_components/labtether` directory into your Home Assistant `config/custom_components/` directory.

## Configuration

1. Go to **Settings > Devices & Services > Add Integration**
2. Search for "LabTether"
3. On **Connect to LabTether Hub**, enter:
   - your LabTether hub URL (for example `https://lab.local:8443`),
   - an API key (for example the `LABTETHER_OWNER_TOKEN` value),
   - an optional display name.
4. Enable **Ignore TLS certificate errors** only when you intentionally use a self-signed LabTether hub certificate and trust that endpoint.
5. Continue to **Choose What To Import** and review the live preview:
   - asset count,
   - telemetry-capable assets,
   - controllable assets,
   - active alerts,
   - visible source summary.
6. Choose what Home Assistant should create:
   - status entities,
   - telemetry sensors,
   - power switches,
   - `labtether.run_action` service,
   - polling interval.
7. Review the summary and finish setup.

## Options Flow

After setup, open the LabTether integration entry and use **Configure** to update:

- TLS ignore-certificate behavior,
- imported entity categories,
- `labtether.run_action` service availability,
- polling interval.

If the API key becomes invalid, Home Assistant can now drive a reauthentication flow for the LabTether entry instead of forcing remove/re-add.

If the hub URL or other required connection details change, use **Configure** on the integration entry to reconfigure the connection in place.

If a LabTether asset is removed permanently, Home Assistant can now remove its stale device entry cleanly from the device registry.

## Entities

### Hub Device
| Entity | Type | Description |
|--------|------|-------------|
| Status | Binary Sensor | Hub reachability |
| Total Assets | Sensor | Count of all monitored assets |
| Active Alerts | Sensor | Count of firing alerts |

### Per-Asset Devices
| Entity | Type | Description |
|--------|------|-------------|
| Status | Binary Sensor | Online/offline status |
| CPU Usage | Sensor | CPU utilization % (compute assets) |
| Memory Usage | Sensor | Memory utilization % (compute assets) |
| Disk Usage | Sensor | Disk utilization % (compute assets) |
| Power | Switch | Start/stop (VMs and containers only) |

## Services

### `labtether.run_action`

Run any LabTether action from an automation.

```yaml
service: labtether.run_action
data:
  asset_id: "pve-vm-100"
  action: "vm.start"
  connector_id: "proxmox"
```

## Development

```bash
cd integrations/homeassistant
python -m pytest tests/ -v
```
