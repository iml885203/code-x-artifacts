---
name: CodeXArtifacts
description: Turn ideas into artifacts. Keep creating value.
colors:
  primary: "#ad4968"
  primary-deep: "oklch(0.46097675 0.12476191 3.36827942)"
  primary-hover: "oklch(0.39979732 0.11091987 5.45027163)"
  primary-light: "oklch(0.78814786 0.08290537 359.64197897)"
  paper: "#f5f3ed"
  charcoal: "#191d24"
  white: "#ffffff"
typography:
  display:
    fontFamily: "Familjen Grotesk"
    fontSize: "clamp(36px, 4.5vw, 60px)"
    fontWeight: 600
    lineHeight: 1.05
    letterSpacing: "-0.035em"
  body:
    fontFamily: "Familjen Grotesk, Noto Sans TC, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.55
rounded:
  control: "8px"
  field: "10px"
  card: "12px"
  website-control: "4px"
spacing:
  small: "8px"
  medium: "16px"
  large: "32px"
components:
  button-primary:
    backgroundColor: "{colors.primary-deep}"
    textColor: "{colors.white}"
    rounded: "{rounded.field}"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "{colors.white}"
  input:
    backgroundColor: "{colors.white}"
    rounded: "{rounded.field}"
    height: "44px"
  artifact-card:
    backgroundColor: "{colors.white}"
    rounded: "{rounded.card}"
---

# Design System: CodeXArtifacts

## Overview

**Creative North Star: "The work continues"**

The owner-selected A website composition and offset-field
mark connect a warm, editorial public face to a quiet working interface.
Artifacts stay visually primary inside the app; berry marks deliberate actions.
This is a Go-backed product with a bilingual interface, not a static UI proposal.

**Key Characteristics:**
- Artifact-first workspace
- Deliberate berry actions
- Light and dark app themes

## Colors

The `primary` berry is the brand accent on the website and mark. App primary
controls use `primary-deep` with white labels in light mode and `primary-light`
with charcoal labels in dark mode. The deeper action shade and its darker hover
shade preserve the kit's existing 7:1 label-contrast requirement; brand berry is
not substituted for this text-bearing action fill. The source
of truth for full light/dark tokens is `internal/web/static/style.css`; website
surface tokens live in `website/styles.css`. Reading contrast is checked in tests.
Kind-specific colors identify formats, with a distinct icon as well as color.

**The Action Contrast Rule.** Keep the brand accent distinct from the app's
text-safe action shades; use `--star`, `--on-star`, `--star-edge` and
`--star-hover` together. Kit v5 shares those app tokens. Released kit v1–v4
remain immutable and do not inherit the berry update.

## Typography

Familjen Grotesk is self-hosted for the wordmark, website and Latin UI. Noto Sans TC
supports Traditional Chinese; Martian Mono is reserved for actual code. Headlines
use a compact line height and tracking no tighter than -0.035em. UI text starts at
14px, metadata at 12–13px; the website varies scale to distinguish its reading levels.
The sign-in introduction uses the frontmatter display role; the website hero uses
48–78px at desktop, 560 weight and 0.99 line height. Georgia appears only inside
labelled synthetic artifact illustrations. It is not the website's interface or
product display face, and illustration-specific type, colors and corners are not
shared application tokens.

## Layout

The website keeps the approved centered value proposition and interactive six-format
illustrative workspace. The app uses its existing library/sidebar and consistent
viewer bar. Sign-in has a broad product introduction and a narrow account form;
at 899px and below they stack. The compact website preview at 680px and below
uses three columns of 11px labels with 44px minimum tab height. Its illustrative
artwork occupies normal flow below its copy rather than overlapping it.
Validate 320, 375, 768, 1024 and 1440px without page overflow.

## Elevation & Depth

App surfaces use tonal layers and quiet borders. Menus use the existing soft
`--float` shadow. The website's main workspace preview uses one broad soft shadow;
content inside it remains clearly labelled as synthetic illustration.

## Shapes

The mark has two offset polygons: `M3 3H15V9L9 15H3Z` and
`M16 10H22V22H10V16Z` on a 24px field. It takes foreground color from context.
Icon/navigation controls use 8px corners, standard buttons and fields use 10px,
and app cards use 12px. Website action controls use 4px corners. These roles are
not interchangeable. Do not restore the pixel-easel mark.

## Components

Primary app controls use the text-safe berry shades described above, a 1px edge,
10px corners, 16px horizontal padding, a minimum 44px height and 500 weight.
They use the actual `--star-hover` fill on hover; active controls translate down
1px, and disabled controls use half opacity. Focus uses the page's text-colored
2px outline with a 2px offset. Standard inputs share the 44px height and 10px
corners, with 12px horizontal padding and a visible field edge.

Library navigation uses 40px rows, 8px corners and raised tonal selection;
artifact cards use quiet borders, 12px corners and a 4px hover lift that is
removed for reduced motion. These are existing role-specific behaviors, not a
blanket minimum-size claim for every illustrative element.

Existing state transitions use 120–320ms ease-out motion. Radix owns menus and
keyboard behavior.
Theme and language remain in the account menu; sign-in exposes language in its
footer. Keep loading, errors, empty states and recovery paths visible.
The website's radio tabs change the labelled format example without network work.

## Do's and Don'ts

- Do keep product prose and headings named CodeXArtifacts.
- Do keep lowercase `pictor` API/settings identifiers compatible where documented.
- Do respect reduced motion and both light/dark themes.
- Do preserve sandbox, sharing and browser focus behavior during visual work.
- Do label illustration as illustration; do not invent usage or acceptance claims.
- Don't introduce the paused TS/Cloudflare architecture into this release.
