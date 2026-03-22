package collectors

import "github.com/labtether/labtether/internal/assets"

func (d *Deps) AutoLinkPortainerHostsToTrueNASHosts() error {
	return d.autoLinkRunsOnByIdentity(
		func(asset assets.Asset) bool {
			return asset.Source == "portainer" && asset.Type == "container-host"
		},
		func(asset assets.Asset) bool {
			return asset.Source == "truenas" && asset.Type == "nas"
		},
	)
}

func (d *Deps) AutoLinkTrueNASHostsToProxmoxGuests() error {
	return d.autoLinkRunsOnByIdentity(
		func(asset assets.Asset) bool {
			return asset.Source == "truenas" && asset.Type == "nas"
		},
		func(asset assets.Asset) bool {
			return asset.Source == "proxmox" && (asset.Type == "vm" || asset.Type == "container")
		},
	)
}
