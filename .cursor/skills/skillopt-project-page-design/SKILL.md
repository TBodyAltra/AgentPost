---
name: skillopt-project-page-design
description: Design and build academic/research project landing pages in the style of Microsoft SkillOpt (microsoft.github.io/SkillOpt)—single-file HTML, glass nav, hero ledger, numbered sections, scroll reveal, data tables, and lightweight interactive charts. Use when the user asks for a project page, paper site, GitHub Pages landing, or wants to mimic SkillOpt’s visual and information architecture.
---

# SkillOpt-style project page design

Reference: [SkillOpt project page](https://microsoft.github.io/SkillOpt/) · Source: [microsoft/SkillOpt `index.html`](https://github.com/microsoft/SkillOpt/blob/main/index.html)

This skill captures **how that page is built and why it works**, so you can reproduce the pattern for other research or product launches—not the SkillOpt ML method itself.

## When to use this pattern

- One **self-contained** GitHub Pages (or static host) site: single `index.html`, inline CSS, small inline `<script>`, assets in a sibling folder (e.g. `skillopt-assets/`).
- Audience: researchers, engineers, and press who need **scan → understand → cite → clone repo** in under two minutes.
- Avoid heavy frameworks unless you need a CMS; SkillOpt optimizes for **zero build step** and fast deploy.

## Information architecture (top → bottom)

| Block | Purpose |
|-------|---------|
| Fixed topbar | Brand, related-project link, in-page anchors, external Code |
| Hero | One-sentence thesis, primary CTAs, **headline metric card** |
| Media | YouTube embed, then high-res paper teaser figure |
| `01`–`07` sections | Numbered story: idea → method → results → ablations → case study → transfer → BibTeX |
| Footer | Title repeat + Code / Citation links |

Each content section uses the same header rhythm:

1. **Eyebrow** — `01 / Core Idea` (mono, violet, uppercase, letter-spaced)
2. **H2** — short declarative headline (display serif)
3. **Lede** — one paragraph of muted body copy (`max-width` ~740px)

Then section-specific body (cards, table, chart, callout).

## Visual system

### CSS variables (semantic tokens)

Define once on `:root` and use everywhere—do not hard-code hex in components:

- **Surfaces:** `--paper`, `--paper-2`, `--panel`, `--panel-warm`
- **Text:** `--ink`, `--muted`, `--quiet`, `--black`
- **Borders:** `--line`, `--line-strong`
- **Accent palette:** `--blue`, `--teal`, `--violet`, `--red`, `--gold`, `--green`
- **Effects:** `--shadow` (soft indigo-tinted elevation)
- **Fonts:** `--display` (Fraunces), `--serif` (Inter), `--mono` (JetBrains Mono)

### Background and atmosphere

- Page: cool off-white `--paper` plus **3–4 large radial gradients** (violet / amber / pink at low opacity)—adds depth without images.
- Hero: stronger local gradients + `min-height: ~76vh`, two-column grid.
- Cards: white panels, `1px` `--line` border, `14–18px` radius, light shadow; **hover** lifts `-3px` and strengthens border/shadow.

### Typography roles

| Role | Font | Usage |
|------|------|--------|
| Display | Fraunces | `h1`, `h2`, big numbers, brand second half |
| Body | Inter | paragraphs, ledes |
| UI / labels | JetBrains Mono | kickers, chips, buttons, table headers, eyebrows |

### Signature gradients

- **Hero title & brand suffix:** `linear-gradient(110deg, teal → indigo → violet → pink → gold)` with `background-clip: text`.
- **Primary CTA:** indigo → pink pill button.
- **Statement card** (manifesto): diagonal indigo–pink gradient fill, white type, glass chips on top.

### Brand mark

- Inline SVG **Microsoft four-square** (16×16) beside wordmark.
- Split name: first part solid ink, second part **italic gradient text** (`Skill` + `Opt` pattern).

## Layout primitives

### Content width

- `main`: `width: min(1080px, calc(100% - 40px)); margin: 0 auto`
- Hero inner: same max width, `grid-template-columns: ~1fr + ~0.5fr` (copy | ledger)

### Section header (reusable)

```html
<div class="section-header">
  <div class="section-eyebrow">03 / Main Results</div>
  <div>
    <h2>…</h2>
    <p class="section-lede">…</p>
  </div>
</div>
```

- Two-column grid: narrow eyebrow column + wide title column.
- `::before` on header: **56×4px** gradient bar (blue → pink) floating above—section break without heavy dividers.

### Hero ledger (metric sidebar)

Right column card for the **single number** you want remembered (e.g. `52/52`):

- Kicker pill → huge display number + muted denominator → short explanation → **3-stat row** separated by vertical rules.
- Subtle **grid overlay** via `::before` on the card (research / “instrument panel” feel).
- `backdrop-filter` + semi-transparent white = glass card on gradient hero.

### Buttons (CTA hierarchy)

| Class | Look | Use |
|-------|------|-----|
| `.button.primary` | Gradient fill, white text | Main in-page jump |
| `.button.secondary` | White fill, gray border | Paper, video, secondary anchors |
| `.button.tertiary` | Near-black fill | GitHub / code |

All: pill shape (`border-radius: 999px`), mono label, icon slot 16px, hover `translateY(-2px)` + colored shadow.

### Related / companion project

Two placements (same visual language):

1. **Navbar pill** — compact: icon + `RELATED` label + project name (gradient text).
2. **Hero row pill** — wider: tag + bold title + one-line summary + arrow that slides on hover.

Use for cross-linking sister papers (SkillOpt ↔ SkillLens).

## Content block recipes

### 1. Kicker + manifesto + steps (Core Idea)

- **Kicker:** small uppercase pill before `h1` in hero (sets category: “Text-space optimization…”).
- **Manifesto grid:** left = gradient **statement** card (`h3` + paragraph + **chip row** of keywords); right = **2×2 step cards** (mono label + short body).
- Chips: frosted pills on gradient (`rgba(255,255,255,0.16)` background).

### 2. Method panels

- Row of equal **panels** with icon/title + bullet list (Evidence, Minibatches, Bounded edits, Memory).
- Full-width **figure frame** below: bordered white box, optional caption strip in mono.

### 3. Results table

- Wrap in `.table-wrap` (horizontal scroll on narrow viewports).
- Zebra or row hover optional; keep numeric columns right-aligned.
- Lead with table; follow with **comparison frame** if you need charts.

### 4. Method comparison charts (JS-generated)

- Data as JSON in `<script>`; `render*` builds DOM (no chart library).
- Per benchmark: **panel** with title, **delta pill** (`SkillOpt +X.X` in green), **bar stage** with CSS `--h` height per bar.
- Legend: `.legend-chip` with `::before` color square.
- Highlight “ours” bar: thicker border + green glow + value label above bar.

### 5. Ablation block

- Table + adjacent **summary list** (what each control proves).
- Optional second figure for training curves.

### 6. Interactive case study (Skill Evolution)

- Static SVG or div-based **chart** with clickable/hover **points** (`data-index`).
- Side or below **detail panel**: step name, status badge, train vs selection scores, summary, bullet list of edits.
- Pre-select best checkpoint on load; sync active point styling (`.is-active`).
- Narrative caption explaining **gate vs train** (why rejected points matter).

### 7. Transfer grid

- 3–4 **stat cards**: large `+N.N` in display font, short label, one-line explanation.
- Closing **callout** paragraph for nuanced claim (not merely distillation).

### 8. BibTeX

- Dark or bordered `.bibtex-box` with `<pre><code>` and **Copy** button.
- `navigator.clipboard.writeText` + 2s “Copied!” feedback.

### 9. Teaser / video blocks

- Shared `.teaser-showcase`: heading row (small label + `h2` + lede), then figure.
- Video: responsive `iframe` in `.video-frame`.
- Paper figure: note in caption that **horizontal scroll** preserves detail on mobile (`overflow-x: auto` on figure wrapper).

## Navigation and motion

### Fixed topbar

- `position: fixed; z-index: 100; backdrop-filter: blur(16px)`
- Default: very transparent; **`.scrolled`** after `scrollY > 40`: more opaque white, bottom border, tighter padding, light shadow.
- Nav links: muted default; hover → blue text + pink underline (subtle brand accent).

### Scroll reveal

```css
.reveal { opacity: 0; transform: translateY(40px); transition: … }
.reveal.visible { opacity: 1; transform: none; }
```

- Apply `.reveal` to major blocks; `IntersectionObserver` adds `.visible` (threshold 0, `rootMargin` bottom inset).
- **`prefers-reduced-motion: reduce`:** force visible, no transform.

### Smooth scroll

- `html { scroll-behavior: smooth; }` for anchor jumps from hero buttons and nav.

## Accessibility checklist

- `aria-label` on `<header>`, `<nav>`, hero action group, ledger, chart regions.
- `aria-labelledby` linking sections to `h2` ids.
- External links: `target="_blank"` + `rel="noopener"`.
- Interactive chart points: `aria-label` per bar/point; keyboard `focus` handlers mirror `mouseenter`.
- Figures: descriptive `alt` on teaser images; iframe `title`.

## Responsive rules (≤ ~900px)

Collapse to single column, in order:

- Hero grid → stack (ledger below copy).
- Section header grid → stack.
- Manifesto, method grid, steps 2×2 → 1 column.
- Comparison grid 3-col → 1 col.
- Ledger stats 3-col → 1 col (drop vertical dividers).
- Shrink display numbers (`clamp` on hero `h1` already).

## Implementation checklist

When building a new page in this style:

1. [ ] Set `:root` tokens and import **Fraunces + Inter + JetBrains Mono** from Google Fonts.
2. [ ] Build topbar + hero with **one killer metric** in the ledger card.
3. [ ] Place video + static teaser **before** numbered sections (visual hook).
4. [ ] Map your story to **5–7 numbered sections** with consistent headers.
5. [ ] Use chips / steps / panels for scanability; reserve tables for dense numbers.
6. [ ] Add one **interactive** element only where it teaches (evolution chart > decorative animation).
7. [ ] Wire scroll reveal + navbar scroll class + copy button.
8. [ ] Test mobile table scroll and reduced motion.
9. [ ] Deploy as static files; keep repo link and arXiv in hero actions.

## Anti-patterns (what SkillOpt avoids)

- Long unstructured prose walls without eyebrows or ledes.
- Multiple competing hero CTAs without visual hierarchy (primary vs secondary).
- Chart libraries for 6 simple bar columns—CSS `--h` bars are enough.
- Autoplay video with sound; embed is click-to-play via YouTube.
- Hiding the main result until section 3—**ledger card** states it immediately.
- Separate CSS/JS files for a one-pager unless caching demands it (SkillOpt inlines for single-file deploy).

## Quick content formula

For each section, fill this template:

```text
Eyebrow: NN / Short topic name
H2: Verb or claim in plain language (≤ 8 words)
Lede: 2–3 sentences — what this block proves, not how.
Body: 1 visual (figure | table | chart) + optional 1 callout sentence.
```

Hero subtitle follows: **Name + subtitle sentence + mechanism list** (comma-separated phases: rollouts, reflection, edits, gates).

---

**License note:** SkillOpt’s `index.html` is MIT (same repo). Adapt the design system freely; replace branding, colors, and copy for your project.
