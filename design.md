# Design Language — ollama.com + onellama Desktop

Consolidated design reference for the onellama fork. Two render surfaces share
one visual system:

- **ollama.com** — permanently **light** theme (marketing / discovery web).
- **onellama desktop app** — Settings **Appearance** control: **Dark** /
  **Light** / **Automatic** (default). Stored in `store.Settings.Theme`
  (DB column `theme`), resolved as a `.dark` class on `<html>`;
  `Automatic` follows `prefers-color-scheme` (dark-first in practice). The
  default screens are Chat / Apps / Settings; the fork adds Models and MCPs.

> Note: the desktop app is **not** hard-wired to dark. `app/ui/app/src/index.css`
> declares the class-based `@custom-variant dark (&:where(.dark, .dark *));`
> plus `.dark { color-scheme: dark }` / `.light { color-scheme: light }` base
> rules; `index.html` runs a pre-paint bootstrap that flips the class from
> `matchMedia("(prefers-color-scheme: dark)")` before React mounts (covers the
> default Automatic). The Models discovery webview receives the resolved theme
> via `?theme=light|dark`.

---

## 1. Shared foundations (both surfaces)

- **Font stack:** `ui-sans-serif, system-ui, "Segoe UI", sans-serif`.
  No custom webfont on either surface. Website order:
  `ui-sans-serif, system-ui, sans-serif, "Apple Color Emoji", "Segoe UI Emoji",
  "Segoe UI Symbol", "Noto Color Emoji"`.
- **Chrome is neutral** (`neutral`/`gray`/`zinc` ramps). Accent color is used
  sparingly: blue for focus/links on the website; blue-500 focus rings in the app.
- **Radius scale:** `rounded-[6px]` (website tags) · `rounded-md` (8px, app
  badges/segments) · `rounded-lg` (8px, app buttons/inputs/rows) · `rounded-xl`
  (12px, app cards) · `rounded-2xl` (16px, app popovers) · `rounded-full` (pills).
- **Focus:** 2px blue ring. Website: `blue-600` (#2563eb). App:
  `ring-2 ring-blue-500` / `focus:outline-2 outline-blue-500`.
- **Spacing:** 4px base grid (`p-1`…`p-6`), `gap-1.5..4`. Sidebar rows are
  `gap-3`; card padding `p-4..5`; page gutters `p-6`.
- **Icons:** Heroicons. Sidebar uses `24/outline` with `stroke-current`; panels
  use `20/solid`.
- **Motion:** short, eased. `--ease` `cubic-bezier(0.4, 0, 0.2, 1)` on dialog
  345ms; copy hint 320–400ms `cubic-bezier(0.22, 1, 0.36, 1)`; every animation
  collapses under `media (prefers-reduced-motion: reduce)`.

### Consolidated token sheet

Canonical values for the merged system. Dark column = desktop app; Light column
= website (Content surfaces default to the app's own bg, the hero/marketing
numbers are documented in §2).

| Token | Light (web) | Dark (app) |
|---|---|---|
| `--bg-canvas` | `#ffffff` | `#0a0a0a` (`neutral-950`) |
| `--bg-surface` | `#ffffff` | `#171717` (`neutral-900`) |
| `--bg-surface-alt` | `#f5f5f5` (`neutral-100`) | `#262626` (`neutral-800`) |
| `--bg-sidebar` | `#fafafa` (`neutral-50`) | `#0a0a0a/40` (`neutral-950/40`) |
| `--text-primary` | `#000000` | `#fafafa` (`neutral-50`/white) |
| `--text-secondary` | `#737373` (`neutral-500`) | `#a3a3a3` (`neutral-400`) |
| `--text-muted` | `#a3a3a3` (`neutral-400`) | `#a3a3a3` (`neutral-400`) |
| `--border` | `#e5e5e5` (`neutral-200`) | `#262626` (`neutral-800`) |
| `--accent` | `#2563eb` (`blue-600`, links/focus) | `#3b82f6` (`blue-500`, focus) |
| `--font-sans` | `ui-sans-serif, system-ui, sans-serif, …` | `ui-sans-serif, system-ui, "Segoe UI", sans-serif` |
| `--font-rounded` | — | `"SF Pro Rounded", ui-sans-serif, system-ui, "Segoe UI", sans-serif` |
| base type | 16px / 24px | 14–16px |

---

## 2. Website (ollama.com) — permanent light

Source of truth: live site, Tailwind **v3.4.12**. Content is monochrome; rarely
anything but black text on white, neutral grays, `#ddf4ff` accent wash, and
indigo tag pills.

- **Body:** black text, 16px/24px, white background.
- **Hero H1:** 56px / weight 500 / line-height 56px / `letter-spacing: -1.4px`,
  black.
- **Hero sub-copy:** 18px, `neutral-500` (#737373).
- **Nav links:** 18px (line-height 28px), no background; row `margin-left: 24px`.
  Nav shell padding ~`9px 24px`.
- **Primary CTA ("Download"):** pill `radius 9999px`, padding `6px 16px`,
  18px, white on `#262626` (`neutral-800`). Hover darkens.
- **Secondary/"Sign in":** pill, padding `6px 16px`, `bg black/5`.
- **Model cards (search results):** *not* cards — flat list rows separated by
  dividers. Padding 24px/0. Title 24px / 500 / 32px. Description 16px
  (`neutral-500`). No border-radius, no shadow.
- **Tag pills** (tools, thinking, cloud, vision): `indigo-50` bg / `indigo-600`
  text, `radius 6px` (`rounded-md`), padding `2px 8px`, 13px.
- **Semantic tint:** `#ddf4ff` (pale blue) used as accent wash.
- **Motion:** htmx indicators cross-fade 200ms; mascot bob 700ms ease; respect
  `prefers-reduced-motion`.

**Website rules:**
- Black/near-black primary actions, neutral dividers, wide airy rows.
- Blue and indigo are informational (links, focus, tag pills) — never large fills.
- Content is flat: no shadows, no card chrome on listings.

---

## 3. Desktop app — default screens (Chat / Apps / Settings)

Source of truth: `app/ui/app/src`. Tailwind **v4**, Headless UI → Tailwind-UI
catalog primitives in `components/ui/*`.

### App shell — `components/layout/layout.tsx` (`SidebarLayout`)

- Frame: `flex h-screen w-full overflow-hidden dark:bg-neutral-900`.
- Sidebar: collapsible `w-48` (`transition-[width] duration-300`), bg
  `neutral-50 dark:bg-neutral-950/40`, `border-r neutral-200 dark:neutral-800`.
- Top title bar: `h-13`, `bg-white dark:bg-neutral-900`, drag region; title is
  `font-rounded text-md font-medium dark:text-white`, `pl-6` when sidebar open.
- Toggle button: `h-9 w-9 rounded-full hover:bg-neutral-100 dark:hover:bg-neutral-700/75`.

### Sidebar navigation — `components/AppSidebar.tsx`

- Item base:
  `flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm text-neutral-700 hover:bg-neutral-100 dark:text-neutral-100 dark:hover:bg-neutral-800`.
- Active: `bg-neutral-100 dark:bg-neutral-800`.
- Section order: **Chat · Models · MCPs** (top), **Apps · Settings** (bottom).
- Icons: Chat = custom filled glyph (`ChatIcon`), others = Heroicons
  `24/outline` at `h-5 w-5 stroke-current`.

### UI primitives (`components/ui/*`)

- **Button** — `relative isolate inline-flex items-baseline justify-center gap-x-2 rounded-lg border text-base/6 font-medium`; sizing
  `px-3.5 py-2.5 sm:px-3 sm:py-1.5 sm:text-sm/6`; focus `outline-2 outline-blue-500`;
  default solid color **`dark/zinc`** (zinc-900 / zinc-600). Full palette:
  outline / plain / dark-zinc / light / white / zinc / indigo / cyan / red /
  orange / amber / yellow / lime / green / emerald / teal / sky / blue / violet /
  purple / fuchsia / pink / rose.
- **Badge** — `inline-flex items-center gap-x-1.5 rounded-md px-1.5 py-0.5 text-sm/5 font-medium sm:text-xs/5`. Color recipe: `bg-{c}-500/15 text-{c}-700 … dark:bg-{c}-500/10 dark:text-{c}-400`. Default `zinc`.
- **Input** — `rounded-lg`, light: white bg + `before:shadow-sm`, border
  `zinc-950/10` hover `/20`; dark: `bg-white/5`, border `white/10` hover `/20`.
  Focus ring `ring-2 ring-blue-500`. Placeholder `zinc-500`. **Inset shadow root,
  translucent dark bg — do not reimplement with plain borders.**
- **Switch, Fieldset, Slider** — Tailwind UI catalog defaults (zinc/neutral).

### Existing surface patterns (from Chat / Settings / Apps)

- Backgrounds: `bg-white dark:bg-neutral-900` for main; `neutral-50/bg-neutral-950/40`
  only for the sidebar; settings cards `rounded-xl`.
- Text hierarchy: headings `text-sm/16 font-semibold`; body `text-sm`;
  secondary `text-neutral-500 dark:text-neutral-400`.
- Focus everywhere = `outline-blue-500` / `ring-blue-500`. Success `green-500`,
  error `red-500`, warning `amber-500`/`amber-400` (Badge alpha recipes).

---

## 4. The fork's additions — Models & MCPs

Both live behind the same sidebar and share the system primitives. Status of
each pattern vs. the system:

| Aspect | `routes/models.tsx` + `ModelsScreen.tsx` | `routes/mcp.tsx` + `MCPServers.tsx` |
|---|---|---|
| App shell | **Consistent** — uses `SidebarLayout title="Models"` + `AppSidebar` | **Consistent** — uses `SidebarLayout title="MCPs"` |
| Icons | `24/outline` | `20/solid` |
| Tinted icon accents | neutral | neutral |
| Primary action | catalog `Button` (Pull Model) | catalog `Button` (Save Changes) |
| Cards | grid cards `rounded-xl border` `bg-white dark:bg-neutral-900` p-4 | cards `rounded-xl border` (broken = red tint) |
| Status | `Badge color="green"` Ready, neutral progress bar | `Badge` green/red/zinc (Active/Broken/Disabled), no glows |
| Code/mono | digest `font-mono` tiny | command/URL/env mono blocks |
| Theme | follows app theme → webview `?theme=` | follows app theme |

---

## 5. What the design language must NOT incorporate

These patterns in the fork **do not** belong to the system and should be
removed or replaced.

1. **Resurrecting web-page layout in the app.**
   - `routes/models.tsx` re-implаments the shell (`h-screen` frame + fixed
     `w-64` sidebar + "Ollama" title) instead of using `SidebarLayout`. The
     default apps all share `SidebarLayout`; the Models route should do the same
     (`<SidebarLayout title="Models" sidebar={<AppSidebar current="models" />}>`).
     Do **not** keep a second, hard-coded chrome.
   - `mcpservers.tsx` wraps content in `max-w-7xl mx-auto` and responsive
     `md:/lg:` grids. That is website thinking. The desktop is a fixed surface —
     prefer app-native spacing/cards, no full-bleed centered web container.

2. **Rainbow icon tints.** `GlobeAltIcon text-blue-500`, `CpuChipIcon
   text-emerald-500`, `SunIcon text-amber-500`, `MoonIcon text-indigo-400` in
   Models violate the rule "chrome is neutral, accent is spare." Neither the site
   nor the defaults tint sidebar-level icons. Keep icons `text-neutral-*`
   (light) / `text-neutral-*` (dark).

3. **Blue-600 large primary fills.** The Models "Pull Model" button
   (`bg-blue-600 hover:bg-blue-500 rounded-lg shadow-xs`) matches neither the
   app (catalog `Button`, `dark/zinc`) nor the site (neutral-800 pill).
   Blue belongs to focus rings and links only.

4. **Extra surface depth in the main pane.** `bg-neutral-50 dark:bg-neutral-950`
   behind Models content makes it darker than the default (white/neutral-900).
   Main content should stay `white/neutral-900`; `neutral-50/950` is reserved
   for the sidebar.

5. **Manual Light/Dark toggle + `!important` dark repaint of a light site.**
   `ModelsScreen` ships a Sun/Moon toggle that passes `?theme=`; `search.html`
   then re-themes the (permanently light) ollama.com with a block of
   `!important` overrides (`color-scheme: dark !important`,
   `background-color #171717 !important`, `#60a5fa` links, `#1e293b` pill bg…).
   This is a fragile bolt-on, not a design language. The **original override is
   flawed** (see §8) and should be replaced with the refined token-mapped one in
   §8 if the embedded discovery pane is kept.

6. **Glowing status dots.** `h-2.5 w-2.5 rounded-full shadow-{c}-/50 shadow-sm`
   introduces glow effects. Use the app `Badge` alpha recipe
   (`bg-green-500/15 dark:bg-green-500/10`) — no glows.

7. **`focus:ring-neutral-500`** on the MCP JSON `<textarea>`. Every other focus
   in the app (buttons, inputs, badges, dialogs) is blue-500. Use
   `ring-2 ring-blue-500`.

8. **Perma-dark code editor regardless of theme.** The MCP textarea is
   `bg-neutral-900 text-neutral-100` in *light* mode too. A permanent dark
   editor can be a deliberate product choice, but then pair it with the app's
   blue focus ring and existing mono conventions — a lone `neutral-900` block in
   an otherwise light pane reads as broken, not intentional.

9. **Hacker-style key-value pills.** `text-[10px] uppercase font-mono
   tracking-wider` type badges and the `emerald-100/emerald-800` "Ready" pill
   drift from both surfaces. Badges should use the catalog `Badge`
   (`rounded-md`, alpha bg, `text-sm/5 sm:text-xs/5`, `font-medium`).

Good patterns from the additions worth keeping:
- MCP route uses the shared `SidebarLayout` correctly.
- MCPServers composes the catalog `Button` and layout-consistent cards.
- Models' grid gallery is a reasonable desktop pattern (drop the web breakpoints,
  keep the card recipe: `rounded-xl border neutral-200 dark:neutral-800
  bg-white dark:bg-neutral-900`, `p-4`, no shadow).

---

## 6. Defaults to copy everywhere

Canonical snippets for new app surfaces.

**Sidebar item**
```tsx
className="flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm text-neutral-700 hover:bg-neutral-100 dark:text-neutral-100 dark:hover:bg-neutral-800"
// active: "bg-neutral-100 dark:bg-neutral-800"
```

**Primary button (app)**
```tsx
<Button>Action</Button>            // solid dark/zinc, focus outline-blue-500
<Button plain>Secondary</Button>
```

**Error / success / warning (app)**
```tsx
<Badge color="green">Ready</Badge>
<Badge color="red">Broken</Badge>
<Badge color="amber">Warning</Badge>
<Badge color="zinc">Neutral</Badge>
```

**Text input (app)**
```tsx
<InputGroup>
  <MagnifyingGlassIcon data-slot="icon" />  {/* 20/solid, h-5 */}
  <Input type="text" placeholder="Filter…" />
</InputGroup>
```

**Page chrome (app)**
```tsx
<SidebarLayout title="Models" sidebar={<AppSidebar current="models" />}>
  … surface: bg-white dark:bg-neutral-900, p-6 …
</SidebarLayout>
```

**Marketing-style CTA (web)**
```html
Tailwind: rounded-full px-4 py-1.5 text-[18px] bg-neutral-800 text-white hover:bg-neutral-700
```

---

## 7. Merged stylesheet (light web + dark app) — tailwind v4 sketch

Register the shared tokens once; both surfaces consume the same variables.

```css
@import "tailwindcss";

@custom-variant dark {
  @media (prefers-color-scheme: dark) {
    &:not(.light-only, .light-only *) { @slot; }
  }
}

@theme {
  --font-sans: ui-sans-serif, system-ui, "Segoe UI", sans-serif;
  --font-rounded: "SF Pro Rounded", ui-sans-serif, system-ui, "Segoe UI", sans-serif;

  /* canonical surfaces */
  --color-canvas:   var(--bg-canvas);
  --color-surface:  var(--bg-surface);
  --color-surfacealt: var(--bg-surface-alt);
  --color-sidebar:  var(--bg-sidebar);
  --color-ink:      var(--text-primary);
  --color-ink-2:    var(--text-secondary);
  --color-line:     var(--border);
  --color-accent:   var(--accent);

  /* motion */
  --ease-standard: cubic-bezier(0.4, 0, 0.2, 1);
  --ease-out-expo: cubic-bezier(0.22, 1, 0.36, 1);
}

:root {
  /* light = ollama.com web */
  --bg-canvas:     #ffffff;
  --bg-surface:    #ffffff;
  --bg-surface-alt:#f5f5f5;
  --bg-sidebar:    #fafafa;
  --text-primary:  #000000;
  --text-secondary:#737373;
  --border:        #e5e5e5;
  --accent:        #2563eb; /* blue-600: links + focus */
}

.dark {
  /* dark = onellama desktop */
  --bg-canvas:     #0a0a0a;
  --bg-surface:    #171717;
  --bg-surface-alt:#262626;
  --bg-sidebar:    rgba(10, 10, 10, 0.4);
  --text-primary:  #fafafa;
  --text-secondary:#a3a3a3;
  --border:        #262626;
  --accent:        #3b82f6; /* blue-500: focus */
}

@layer base {
  html { color-scheme: light dark; }
  .light-only { color-scheme: light; }
  *:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
}
```

Usage rules for the merged system:
- Never hardcode `#fff`/`#000` text or `blue-600` fills in the app; use the
  tokens.
- Keep chrome neutral; accent = focus/links only (web) or focus only (app).
- One radius ladder, one mono ramp for code, one eased motion vocabulary.

---

## 8. Dark theme for the embedded models page (webview)

The Models discovery tab embeds **olloama.com's own pages** (a light-only site)
through a local proxy (`search.html` → `/api/v1/models/webview?path=…&theme=…`).
Because the site is permanently light, a dark pane must be produced by an
override stylesheet. This section documents the findings from live experiments
against `/search` and `/library/llama3.2`, and gives the canonical override.

### The original override (`search.html` `onellama-theme`) — why it was replaced

The shipped block was a blunt repaint that violated the design system:

- Forced **every** link `#60a5fa` blue — including ordinary nav/model links that
  must stay near-white. Only *distinct* accent elements should read blue.
- Hardcoded off-palette colors: `#1e293b` (slate-800) pills and `#333333` borders
  do not exist anywhere in the app or site ramp.
- Collapsed the tag system: both the indigo `tools` pill and the `#ddf4ff` size
  pills became identical slate boxes with borders, destroying the semantic
  indigo-vs-blue distinction.
- Covered only a subset of classes, so the **model detail page** leaked styled
  light surfaces (`prose-code:bg-neutral-100`, `md-*` markdown elements,
  `text-gray-*`, `text-green-700`, `bg-neutral-50`).

### Palette mapping (light site → dark override)

Token-mapped, semantic colors preserved, matching the app's neutral ladder plus
the `Badge` alpha recipe (§3) for tags:

| Site light | Dark replacement | Rationale |
|---|---|---|
| `bg-white` / `bg-neutral-50` | `#171717` / `#131313` | app `--bg-surface` ladder |
| `text-black` / `text-neutral-900` | `#f5f5f5` | `--text-primary` |
| `text-neutral-800` / `-700` | `#d4d4d4` | secondary heading text |
| `text-neutral-500` / `-400` | `#a3a3a3` | `--text-secondary` |
| `text-gray-500` / `-600` | `#a3a3a3` | same ramp (site mixes gray+neutral) |
| `text-gray-400` | `#9ca3af` | muted |
| `bg-neutral-800` (CTA/search) | `#404040` → hover `#525252` | lifted button off surface |
| `bg-black/5` (pill) | `rgba(255,255,255,0.04)` | inverted surface tint, not flat |
| `bg-indigo-50` + `text-indigo-600` | `rgba(99,102,241,0.15)` + `#c7d2fe` | `Badge` tools recipe, tint kept |
| `bg-[#ddf4ff]` + `text-blue-600` | `rgba(59,130,246,0.15)` + `#93c5fd` | `Badge` size recipe, tint kept |
| `text-green-700` | `#6ee7b7` | semantic green keeps meaning |
| `pre` / `code` / `md-*` | `#1a1a1a` + `#d4d4d4` | code surface, one mono ramp |
| `input` / `select` / `textarea` | `#171717` + `#333` + `#e5e5e5` | controls on surface |
| `border-neutral-100/200`, `gray-200` | `#333` | borders one step off surface |
| focus `ring-blue-300/400` | target `#3b82f6` (`blue-500`) | app focus color |

### Canonical override (`search.html` `onellama-theme` replacement)

Verified live against `/search?q=llama` and `/library/llama3.2`: after
injection a computed-style sweep reports **zero opaque light backgrounds**; the
only light pixels left are the intentional `rgba(255,255,255,0.04)` pill tint.

```css
/* ---------- onellama dark theme for ollama.com (webview) ---------- */
/* Canonical override — keep in sync with server/models_registry.go, app/ui/models_registry.go, search.html */
:root { color-scheme: dark; }

/* Canvas */
html, body, main { background-color:#0a0a0a !important; color-scheme:dark !important; }
body, main { color:#f5f5f5 !important; }

/* Header shell */
header, header.sticky, header.bg-white, .bg-white { background-color:#171717 !important; border-color:#333 !important; }
header nav a, header span { color:#e5e5e5 !important; }

/* Primary / secondary text ramp */
.text-black, .text-neutral-900, h1, h2, h3, h4, h5, h6, strong, b, .prose, .prose * { color:#f5f5f5 !important; }
.text-neutral-800 { color:#d4d4d4 !important; }
.text-neutral-700 { color:#d4d4d4 !important; }
.text-neutral-600, .text-neutral-500, .text-neutral-400, .text-gray-500, .text-gray-600 { color:#a3a3a3 !important; }
.text-gray-400 { color:#9ca3af !important; }
.text-white { color:#fff !important; }

/* Semantic accents — site blue/indigo/green kept but lightened for dark bg */
.text-blue-600 { color:#93c5fd !important; }
.text-indigo-600 { color:#c7d2fe !important; }
.text-green-700 { color:#6ee7b7 !important; }
.bg-indigo-50 { background-color:rgba(99,102,241,0.15) !important; }   /* tools pill */
.bg-\[#ddf4ff\] { background-color:rgba(59,130,246,0.15) !important; } /* size pill */

/* Surfaces */
.bg-neutral-50 { background-color:#131313 !important; }
.bg-black\/5, div[class*="bg-black/5"] { background-color:rgba(255,255,255,0.04) !important; }
.bg-neutral-800 { background-color:#404040 !important; color:#fff !important; }
a.bg-neutral-800:hover, a.bg-neutral-800:focus, .focus\:bg-black { background-color:#525252 !important; }

/* Filter chips — unselected stays transparent with neutral ink; selected lifts */
[class*="bg-black"] { background-color:rgba(255,255,255,0.04) !important; color:#b3b3b3 !important; }
[class*="bg-neutral-800"], a[class*="bg-neutral-800"] { background-color:rgba(255,255,255,0.08) !important; color:#f5f5f5 !important; border-color:#333 !important; }
a[class*="bg-neutral-800"]:hover, a[class*="bg-neutral-800"]:focus { background-color:rgba(255,255,255,0.12) !important; }

/* Code / prose */
pre, code, .prose-pre pre, .prose-code code, code\:bg-gray-200 { background-color:#1a1a1a !important; color:#d4d4d4 !important; }
md-pre, md-code, md-bold, md-italic { color:#d4d4d4 !important; }

/* Detail page lists / bullets surfaced as plain spans */
.prose ul, .prose ol, .prose li, ul, ol, li, [class*="markdown"], .md-text { color:#e5e5e5 !important; }

/* Controls */
input, select, textarea { background-color:#171717 !important; border-color:#333 !important; color:#e5e5e5 !important; }
input::placeholder, textarea::placeholder { color:#6b7280 !important; }
select:hover, option { background-color:#1c1c1c !important; color:#e5e5e5 !important; }
.border-neutral-100, .border-neutral-200, .border-gray-200 { border-color:#333 !important; }

/* Scrollbar */
::-webkit-scrollbar { width:10px; height:10px; }
::-webkit-scrollbar-thumb { background:#333; border-radius:8px; }
```

### Rules for any future repaint of a light site in the app

- **Never blanket-recolor `a`** — only the semantic accent (blue/indigo/green)
  utilities; nav and content links stay `#f5f5f5`/`#e5e5e5`.
- Always invert `bg-black/5` → `rgba(255,255,255,.04)`; a flat dark fill breaks
  the surface rhythm the site is built on.
- Keep the indigo-vs-blue tag distinction (`rgba(99,102,241,.15)` vs
  `rgba(59,130,246,.15)`); never collapse them into one slate box.
- Cover detail pages too (`prose-*`, `md-*`, `text-gray-*`, `code:bg-gray-200`,
  `ul`/`ol`/`li` bullets), not just the listing page.
- Treat search filter chips like pills: unselected stays a faint
  `rgba(255,255,255,.04)` tint, selected lifts to `rgba(255,255,255,.08)`,
  hover `.12` — never a flat slate fill.
- Verify with a computed-style sweep (no opaque `rgb(2xx,2xx,2xx)` background
  except the intentional white-alpha tint), since screenshots aren't objective.
- The override lives in three places and **must stay byte-identical**:
  `server/models_registry.go`, `app/ui/models_registry.go`, and the static
  `search.html`. Keep `?theme=light|dark` toggling intact and let light mode
  fall back to the unmodified site.