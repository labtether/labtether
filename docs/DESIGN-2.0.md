# LabTether 2.0 design

How the Hub console, the website and the macOS, Windows and iOS apps look in
2.0. The console is the reference. Native apps keep their own controls and
system font, dressed in the console's colours, sizes and list layout.

## Where the values live

| File | What it is |
|---|---|
| `web/console/app/styles/themes.css` | The source: every colour (light and dark), radius, control and row height, type size |
| `web/console/app/styles/foundation.css` | Fonts, heading weights and the Tailwind radii |
| `web/console/design/tokens.json` | Every value above, read from the CSS, with the hub commit it came from. Generated |
| `LTTokens.swift` | SwiftUI colours that follow light and dark mode, radii, sizes, type scale (macOS 13+, iOS 17+). Generated |
| `LabTetherTheme.xaml` | WinUI 3 `ResourceDictionary` with Light and Dark theme dictionaries. Generated |
| `tokens.css` | The same values as CSS custom properties for the website. Generated |

Change a value in `themes.css`, never in a generated file. Then, from the
LabTether workspace root (maintainers):

```sh
python3 scripts/design/sync-tokens.py           # writes tokens.json and the app files
python3 scripts/design/sync-tokens.py --check   # reports drift, writes nothing
```

The Swift, XAML and CSS files are written to
`work/design-2.0-rollout/tokens-out/` until each app adopts them. `--check`
writes nothing; it exits 1 and lists every value that differs, including the
names the website's `app/styles/foundation.css` shares with the console. Run it
before each pull request that changes tokens.

Not shared: shadows, motion and blur stay native to each platform. The
console's optional blue accent (Deep Signal) is only in `tokens.json`.

## Colour roles

| Token | Use |
|---|---|
| `canvas` | Window background |
| `panel` | Sheets: lists, detail panels, forms |
| `sunk` | Wells inside a sheet: code blocks, expanded rows |
| `text`, `text-2`, `text-3` | Primary, secondary, quiet text. All pass WCAG AA 4.5:1 |
| `divider` | The one hairline: under a column header, above a sheet footer |
| `hover`, `sel` | Row hover; selected row and pressed control |
| `rose` (`accent`) | The one main button on a screen. Nothing else |
| `ok`, `warn`, `bad` | Status dots, meter fills, chart warnings |
| `warn-text`, `bad-text` | Problem words |
| `chart`, `chart-fill` | The single chart colour and its flat fill |
| `track` | Meter and switch track |
| `term-bg`, `term-text` | Terminal and log views |

Light and dark follow the system setting.

## Rules

- **Rose:** only the main button, at most one per screen. Switches,
  checkboxes and selected tabs use the text colour.
- **Shape:** the window is the canvas; content sits on sheets. Radius `r-2`
  (6) for rows, `r-3` (8) for controls, `r-4` (12) for sheets. No 1 px boxes,
  no sheet inside a sheet, no row dividers.
- **Sizes:** controls `ctl` 32 (28 inside rows), rows `row-h` 36, two-line
  rows 48.
- **Lists first:** one row per thing, like Tailscale or Linear. The name, then
  a second line in `text-3`. A problem replaces the second line in `warn-text`
  or `bad-text`. Usage bars are thin and turn amber from 85 %.
- **Status dot:** only when something is not OK. A healthy row has no dot.
- **Status words:** a word plus a time: "Offline since 21:12",
  "Recovered at 14:05".
- **Type:** the console's scale: title 24, head 16, body 14, meta 13, micro
  12. Native apps use the system font (SF Pro, Segoe UI Variable); the web uses
  Inter. Mono only for paths, code and logs, never for IP addresses or times.
- **Icons:** outlined, never in a coloured box. Each device kind keeps the
  console's meaning: hypervisor `Server`, NAS `HardDrive`, VM `AppWindow`,
  container host `Container`, Mac `Laptop`, Windows `Monitor`, anything else
  `Computer` (lucide names). Native apps use the closest SF Symbol or Segoe
  Fluent icon.
- **Banned:** gradients, glows, colour washes, glass cards, left accent bars,
  KPI tiles, stat strips, emoji, rainbow charts, raw IDs in normal views.
- **Words:** the No AI slop rules in `AGENTS.md`. Use the console's page
  names: Dashboard, Devices, Map, Services, Containers, Alerts, Automations,
  Logs, Terminal, Files, Remote View, Settings. Labels are a verb or a noun
  ("Start VM"). No hype, filler or guesses.

## Examples

A device row with a problem, then the main button.

SwiftUI:

```swift
VStack(alignment: .leading, spacing: 2) {
    Text("pve-01").font(LTTokens.TypeScale.body.font.weight(.medium))
    Text("Offline since 21:12")
        .font(LTTokens.TypeScale.meta.font)
        .foregroundStyle(LTTokens.Colors.badText)
}
.frame(maxWidth: .infinity, minHeight: LTTokens.Size.rowH, alignment: .leading)
.padding(.horizontal, 12)
.background(LTTokens.Colors.panel, in: RoundedRectangle(cornerRadius: LTTokens.Radius.r4))

Button("Add device") {}
    .buttonStyle(.borderedProminent)
    .tint(LTTokens.Colors.rose)
```

WinUI 3 (merge `LabTetherTheme.xaml` in `App.xaml`, after `XamlControlsResources`):

```xml
<Border Background="{ThemeResource LTPanelBrush}" CornerRadius="{StaticResource LTRadius4}" Padding="12,0">
    <StackPanel MinHeight="{StaticResource LTSizeRowH}" VerticalAlignment="Center">
        <TextBlock Text="pve-01" Style="{StaticResource LTBodyTextStyle}" Foreground="{ThemeResource LTTextBrush}" />
        <TextBlock Text="Offline since 21:12" Style="{StaticResource LTMetaTextStyle}"
                   Foreground="{ThemeResource LTBadTextBrush}" />
    </StackPanel>
</Border>
<Button Content="Add device" Background="{ThemeResource LTRoseBrush}" Foreground="White"
        CornerRadius="{StaticResource LTRadius3}" Height="{StaticResource LTSizeCtl}" />
```

CSS:

```css
.row { min-height: var(--row-h); padding: 0 12px; border-radius: var(--r-2); }
.row:hover { background: var(--hover); }
.row .problem { color: var(--bad-text); font-size: var(--fs-meta); line-height: var(--lh-meta); }
.btn-main { height: var(--ctl); border-radius: var(--r-3); background: var(--rose); color: var(--accent-contrast); }
```
