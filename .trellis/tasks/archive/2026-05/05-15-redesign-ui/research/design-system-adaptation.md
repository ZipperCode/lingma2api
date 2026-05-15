# DESIGN.md Adaptation Notes

## Source

- Root design document: `DESIGN.md`

## Core Tokens

- Canvas: `#faf9f5`
- Surface soft: `#f5f0e8`
- Surface card: `#efe9de`
- Surface cream strong: `#e8e0d2`
- Primary coral: `#cc785c`
- Primary active: `#a9583e`
- Hairline: `#e6dfd8`
- Ink: `#141413`
- Body: `#3d3d3a`
- Muted: `#6c6a64`
- Dark surface: `#181715`
- Dark elevated: `#252320`
- Dark soft: `#1f1e1b`

## Typography Decision

- Use Cormorant Garamond for display headings as the closest open-source substitute for Copernicus / Tiempos Headline.
- Use Inter for body, navigation, buttons, labels, forms.
- Use JetBrains Mono for code viewers and JSON/log content.
- Display headings should remain regular/medium weight with negative letter spacing rather than bold sans-serif.

## Console Mapping

- Existing `sidebar` maps to a console-appropriate variation of DESIGN.md `top-nav`: warm cream surface, hairline borders, editorial wordmark treatment, coral active states.
- Existing `.card`, `.stat-card`, tables, forms, tabs, modals map to DESIGN.md content cards, feature cards, inputs, category tabs, and overlays.
- Code/log viewers should use dark product surface tokens to create the cream-to-dark rhythm described in DESIGN.md.
- Keep existing product functionality and routing unchanged; implement style changes primarily through CSS and minimal class cleanup.

## Implementation Guardrails

- Remove cool blue/purple gradients and glassmorphism as the dominant style.
- Avoid pure white canvas and cool gray UI floors.
- Use coral sparingly: primary buttons, active navigation/tabs, key inline links, and selected indicators.
- Prefer color-block depth and hairline borders over heavy shadows.
- Preserve responsive behavior of the existing dashboard/control-panel layout.
