# Third-party notices

LabTether ships files from the projects below. Product names, logos and
trademarks belong to their owners. LabTether uses each logo only to label the
matching service and does not claim any endorsement.

## dashboard-icons

- Project: [dashboard-icons](https://github.com/homarr-labs/dashboard-icons)
  by Homarr Labs, formerly `walkxcode/dashboard-icons`.
- License: Apache License 2.0. The full text ships next to the icons in
  [`web/console/public/service-icons/LICENSE-dashboard-icons.txt`](web/console/public/service-icons/LICENSE-dashboard-icons.txt).
- Files: the SVG icons in `web/console/public/service-icons/`, fetched by
  [`scripts/fetch-dashboard-icons.sh`](scripts/fetch-dashboard-icons.sh),
  except the files listed in the sections below.
- Upstream notice: "All product names, trademarks, and registered trademarks
  are the property of their respective owners. Icons are used for
  identification purposes only and do not imply endorsement."

LabTether changed these dashboard-icons files. It removed the Inkscape
export file paths and left the drawings as they were:

- `ai-on-the-edge-device.svg`
- `ciphermail.svg`
- `dia.svg`
- `digi-kam.svg`
- `doplarr.svg`
- `electrum.svg`
- `garuda-linux.svg`
- `openwebrx-plus.svg`
- `openwebrx-plus-dark.svg`

`ariang.svg` holds the dashboard-icons file `png/ariang.png`, scaled down to
128 × 128 pixels and placed inside an SVG file.

Upstream removed its ServiceNow icons on 2026-04-29 in commit
[`278a048a`](https://github.com/homarr-labs/dashboard-icons/commit/278a048a39637f97d8765252fb389246fae62c93).
LabTether still ships the copies fetched before that date under the same
license: `servicenow.svg`, `servicenow-wordmark-dark.svg` and
`servicenow-wordmark-light.svg`.

## selfh.st icons

- Project: [selfh.st icons](https://github.com/selfhst/icons) by selfh.st.
- License: Creative Commons Attribution 4.0 International (CC BY 4.0). The
  full text ships next to the icons in
  [`web/console/public/service-icons/LICENSE-selfhst-icons.txt`](web/console/public/service-icons/LICENSE-selfhst-icons.txt).
- Source: commit
  [`7141ddbb`](https://github.com/selfhst/icons/tree/7141ddbb130a5245148a2c7351289dfd8054f157).

Copied without change, from `svg/`: `aria2.svg`, `docmost.svg`, `go2rtc.svg`,
`homepage.svg`, `sftpgo.svg`, `technitium.svg`.

Changed by LabTether. Each file holds the selfh.st PNG named after it, placed
inside an SVG file. "Scaled" means it was also scaled down to 128 pixels
high.

- `dashy.svg` from `png/dashy.png`, scaled
- `flame.svg` from `png/flame.png`, scaled
- `jdownloader.svg` from `png/jdownloader.png`, scaled
- `meshcentral.svg` from `png/meshcentral.png`, scaled
- `mylar3.svg` from `png/mylar3.png`
- `scrypted.svg` from `png/scrypted.png`, scaled
- `tdarr.svg` from `png/tdarr.png`, scaled
- `unmanic.svg` from `png/unmanic.png`

## Simple Icons

- Project: [Simple Icons](https://github.com/simple-icons/simple-icons).
- License: CC0 1.0 Universal.
- Files in `web/console/public/service-icons/`: `amp.svg`, `drone.svg`,
  `wyze.svg`.
- `wg-easy.svg` is the Simple Icons `wireguard.svg` with its fill set to
  `#88171A`, the WireGuard colour listed by Simple Icons.

## Logos from the projects' own repositories

Each file below is copied without change from the project's repository. Its
license text ships in `web/console/public/service-icons/`.

| File | Source | License | Copyright | License text |
|---|---|---|---|---|
| `deconz.svg` | [dresden-elektronik/deconz](https://github.com/dresden-elektronik/deconz/blob/d9bf0bd84da11d8d7db4666ba101039c25a63d8c/icons/de.svg) `icons/de.svg` | BSD 3-Clause | Copyright (c) 2024, dresden elektronik ingenieurtechnik gmbh | [`LICENSE-deconz.txt`](web/console/public/service-icons/LICENSE-deconz.txt) |
| `zwave-js-ui.svg` | [zwave-js/zwave-js-ui](https://github.com/zwave-js/zwave-js-ui/blob/d409051b9e96939fa0510399e45693b331a34cf1/public/logo.svg) `public/logo.svg` | MIT | Copyright (c) 2021 Daniel Lando | [`LICENSE-zwave-js-ui.txt`](web/console/public/service-icons/LICENSE-zwave-js-ui.txt) |
| `organizr.svg` | [causefx/Organizr](https://github.com/causefx/Organizr/blob/6b27515ff3bc6f4998ed83aca8c08be0998fd118/plugins/images/favicon/safari-pinned-tab.svg) `plugins/images/favicon/safari-pinned-tab.svg` | GPL-3.0 | The Organizr repository states no copyright line. | [`LICENSE-GPL-3.0.txt`](web/console/public/service-icons/LICENSE-GPL-3.0.txt) |
| `ntopng.svg` | [ntop/ntopng](https://github.com/ntop/ntopng/blob/d06ae73e5cdbd1564636c05fd246bf0fd8f65cb5/assets/images/logo-square.svg) `assets/images/logo-square.svg` | GPL-3.0 | (C) 2013-26 - ntop.org, as stated in the ntopng source files | [`LICENSE-GPL-3.0.txt`](web/console/public/service-icons/LICENSE-GPL-3.0.txt) |

`organizr.svg` and `ntopng.svg` are GPL-3.0 files bundled alongside LabTether,
which is licensed under the Apache License 2.0. These two files stay under the
GNU General Public License version 3. Each SVG file is its own complete source
code.

## Logo from a project website

`dbgate.svg` is copied without change from the DbGate website:
`https://dbgate.io/img/LOGO-B-simple-gradient.svg`. The website states no
license for this file.

## LabTether's own icons

`_default.svg`, `pairdrop.svg` and `streammaster.svg` in
`web/console/public/service-icons/` are LabTether's own. `pairdrop.svg` and
`streammaster.svg` show the first letter of the service name, not a project
logo.
