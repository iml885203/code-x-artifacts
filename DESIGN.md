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
  website-ink: "#211e20"
  website-muted: "#625b60"
  website-line: "#d8d1d1"
  website-warm-white: "#fbfaf6"
  website-berry-deep: "#8e3451"
  website-berry-pale: "#f5e8ed"
  website-stage: "#eee7e8"
  website-rail: "#20252d"
  website-panel: "#282e38"
  website-panel-line: "#454d59"
  website-panel-copy: "#c3c9d2"
  website-dark-paper: "#201c20"
  website-dark-ink: "#f5f0ed"
  website-dark-muted: "#c0b4bc"
  website-dark-line: "#4c4149"
  website-dark-surface: "#2b242b"
  website-dark-stage: "#352830"
  website-dark-accent-text: "#e4a7bc"
typography:
  display:
    fontFamily: "Familjen Grotesk"
    fontSize: "clamp(36px, 4.5vw, 60px)"
    fontWeight: 600
    lineHeight: 1.05
    letterSpacing: "-0.035em"
  website-brand:
    fontFamily: "Familjen Grotesk, sans-serif"
    fontSize: "clamp(68px, 7.4vw, 104px)"
    fontWeight: 700
    lineHeight: 0.83
    letterSpacing: "-0.04em"
  website-hero:
    fontFamily: "Familjen Grotesk, sans-serif"
    fontSize: "clamp(26px, 2.6vw, 38px)"
    fontWeight: 450
    lineHeight: 1.16
    letterSpacing: "-0.025em"
  website-heading:
    fontFamily: "Familjen Grotesk, sans-serif"
    fontSize: "clamp(32px, 4.3vw, 58px)"
    fontWeight: 500
    lineHeight: 1.02
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
  website-action: "999px"
  website-tab: "7px"
  website-workspace: "16px"
  website-specimen: "14px"
  website-section: "36px"
  website-closing: "28px"
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
  website-button:
    backgroundColor: "{colors.website-ink}"
    textColor: "{colors.paper}"
    rounded: "{rounded.website-action}"
    padding: "13px 24px"
  website-button-secondary:
    backgroundColor: "{colors.website-stage}"
    textColor: "{colors.website-ink}"
    rounded: "{rounded.website-action}"
    padding: "13px 24px"
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

The public website expresses a berry artifact studio: oversized neutral type,
warm paper and scattered authored specimens connect the offset-field mark to
a spacious editorial public face. The app retains its quiet working interface.
Artifacts stay visually primary inside the app; berry marks deliberate actions.
This is a Go-backed product with a bilingual interface, not a static UI proposal.

**Key Characteristics:**
- Artifact-first workspace
- Deliberate berry actions
- Light and dark themes with independently scoped website surfaces

## Colors

The `primary` berry is the brand accent on the website and mark. App primary
controls use `primary-deep` with white labels in light mode and `primary-light`
with charcoal labels in dark mode. The deeper action shade and its darker hover
shade preserve the kit's existing 7:1 label-contrast requirement; brand berry is
not substituted for this text-bearing action fill. The source
of truth for full light/dark tokens is `internal/web/static/style.css`; website
surface tokens live in `website/styles.css`. Reading contrast is checked in tests.
The website defaults to warm paper with charcoal text; its theme switch changes
only the website stage. Dark website tokens are separately named in frontmatter.
The screenshot selector remains dark in both themes. Its images retain the actual
application light theme captured from the local instance; switching the website
theme does not recolor or reconstruct the product screenshots. The closing invitation also keeps its fixed deep berry fill.
Kind-specific colors identify formats, with a distinct icon as well as color.

**The Action Contrast Rule.** Keep the brand accent distinct from the app's
text-safe action shades; use `--star`, `--on-star`, `--star-edge` and
`--star-hover` together. Kit v5 shares those app tokens. Released kit v1–v4
remain immutable and do not inherit the berry update.

## Typography

Familjen Grotesk is self-hosted for the wordmark, website and Latin UI. Noto Sans TC
supports Traditional Chinese; Martian Mono is reserved for actual code. App headlines
use a compact line height and tracking no tighter than -0.035em. UI text starts at
14px, metadata at 12–13px; the website varies scale to distinguish its reading levels.
The sign-in introduction uses the frontmatter display role. The website's two-line
CodeX / Artifacts brand composition uses the website-brand role (including its
website-only -0.04em tracking); the actual h1 uses website-hero, with its second
line at 650 weight. Website section headings use website-heading. At 900px and
below the brand uses clamp(68px, 14vw, 96px), the h1 is 28px, demo heading 38px,
and workflow/formats headings 42px. Product screenshots preserve their original
rendered typography; screenshot content is not a source of website style tokens.

## Layout

The website uses a 1440px maximum container with clamp(22px, 5vw, 72px)
horizontal padding. A centered two-line brand and value proposition sit among
rotated Doc, Slides and Board screenshots. A dark five-type product screenshot
selector follows, then asymmetric two-column workflow and format sections and
a berry closing invitation. This replaces the previous A website composition;
it does not redefine the application layout or released kits.

At 1100px and below specimens tighten while the main screenshot retains its full
3:2 proportions. At 900px and below specimens enter normal flow
in two columns, the small board disappears, preview tabs become three columns
with 44px minimum height, and preview/workflow/formats become single-column.
Tabs use 13px labels; screenshot captions remain at least 11px. The app
retains its existing library/sidebar and viewer bar; sign-in stacks at 899px.
Validate 320, 375, 768, 1024 and 1440px without page overflow.

## Elevation & Depth

App surfaces use tonal layers and quiet borders. Menus use the existing soft
`--float` shadow. Website depth belongs to the floating screenshot images through
`0 20px 45px #33242b24`. The main screenshot selector has no shadow. Captions
identify actual application screenshots with purpose-made demonstration content.

## Shapes

The mark has two offset polygons: `M3 3H15V9L9 15H3Z` and
`M16 10H22V22H10V16Z` on a 24px field. Standalone brand marks on the
website, README, sign-in page, favicon and share assets use brand berry
`#AD4968`; the wordmark keeps its neutral foreground. Product navigation uses
monochrome marks that follow the current theme. Product screenshots preserve the captured application marks and theme. Sign-in reuses the fixed
brand token `--mark-amber` for its mark, independently of action shades.
Icon/navigation controls use 8px corners, standard buttons and fields use 10px,
and app cards use 12px. Website actions and GitHub navigation use pill corners; the theme switch and
scroll control are circles. Website tabs use 7px, screenshot selector 16px,
main image 8px and floating screenshot images 9px, section shells 36px
(25px compact), and closing shell 28px (20px compact). These website surface
roles do not replace the app's 8/10/12px geometry. These roles are not interchangeable. Do not restore the pixel-easel mark.

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
The website's native radio tabs select five actual displayed types: Web page, Doc,
Slides, Design and Board. HTML and React belong to Web page. Screenshot images
are static WebP files displayed in place, with no image links or new-tab action.
Specimen and format links select the radio, focus it, and scroll to the workspace.
The website theme button updates its accessible label and pressed state, starts
in light mode and does not persist or change the account theme. Website actions
have a 48px minimum height and 3px berry focus outline with 5px offset.

Website easing is cubic-bezier(.19, 1, .22, 1): buttons lift 3px over 250ms,
specimen hover rotation/lift takes 600ms, and screenshot arrival takes 300ms.
Fine-pointer hero movement is requestAnimationFrame-bounded to 6/-8/4px per
specimen, resets on pointer leave or preference change, and has no ongoing loop.
Reduced motion removes transitions, animation, smooth scrolling and specimen
translation. Touch and coarse pointers do not trigger depth.

The website's agent section sits between product screenshots and the workflow.
Five semantic buttons name Claude Code, Codex, Cursor, OpenCode and Any MCP client.
Native SVG paths connect their measured positions through the MCP protocol label to
CodeXArtifacts; the SVG is decorative and the labels remain real, selectable text.
This is a connection illustration, never a live status panel or orchestration claim.
The section uses existing website tokens: 28px map shell (22px compact), 12px client
buttons, 14px hub and 18px destination. Client controls are 70px high (54px compact).
At 900px and below clients form a two-column group with the fifth centered; paths
route through side gutters so they do not cross labels.

Client selection highlights its path, updates aria-pressed and announces a short
connection explanation. The setup link uses the README's connect-your-agent section.
Berry packets run along paths over 4s with staggered delays, only while the section
is visible and the document active. A pause/resume button preserves manual choice;
reduced motion removes packet animation and hides that unnecessary control. No
animation library or Astryx dependency is added; the reference informed the interaction.

## Do's and Don'ts

- Do keep product prose and headings named CodeXArtifacts.
- Do keep lowercase `pictor` API/settings identifiers compatible where documented.
- Do respect reduced motion and both light/dark themes.
- Do preserve sandbox, sharing and browser focus behavior during visual work.
- Do distinguish actual application screenshots from their demonstration content.
- Do keep website categories aligned with displayedKinds and translated product labels;
  do not list HTML and React as separate UI types or advertise reserved kinds.
- Don't introduce the paused TS/Cloudflare architecture into this release.
