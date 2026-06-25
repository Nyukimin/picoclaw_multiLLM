---
version: 0.1.0
brand:
  name: RenCrow IdleChat
  product: RenCrow Viewer
  source_of_truth: picoclaw_multiLLM
  design_direction: white_sci_fi_fantasy_lab
  reference_image:
    description: "User-provided IdleChat concept image, June 25 2026"
tokens:
  color:
    background:
      lab: "#F6FAFF"
      lab_soft_blue: "#EAF4FF"
      lab_lavender: "#F1ECFF"
      glass: "rgba(255, 255, 255, 0.72)"
      glass_strong: "rgba(255, 255, 255, 0.86)"
      overlay: "rgba(236, 246, 255, 0.58)"
    text:
      primary: "#17285C"
      secondary: "#4F6091"
      muted: "#7B8AB7"
      inverse: "#FFFFFF"
    accent:
      crystal_blue: "#4F8DFF"
      cyan_hologram: "#6EDCFF"
      soft_lavender: "#B79CFF"
      pale_gold: "#DDBE73"
      mio: "#BFA7FF"
      shiro: "#6D89C9"
    state:
      live: "#3156D4"
      ok: "#40B97A"
      running: "#5A9DFF"
      warning: "#D9A441"
      error: "#D58A45"
      offline: "#9CA8C5"
    border:
      default: "rgba(110, 160, 235, 0.30)"
      strong: "rgba(79, 141, 255, 0.42)"
      soft: "rgba(255, 255, 255, 0.66)"
  typography:
    family:
      ui: '"Noto Sans JP", "Inter", "Segoe UI", sans-serif'
      display: '"Noto Serif JP", "Yu Mincho", "Times New Roman", serif'
    size:
      caption: "11px"
      body: "13px"
      body_large: "15px"
      panel_title: "16px"
      brand: "30px"
    weight:
      regular: 400
      medium: 600
      bold: 700
  radius:
    chip: "999px"
    control: "10px"
    card: "14px"
    panel: "20px"
    stage: "28px"
  spacing:
    xxs: "4px"
    xs: "6px"
    sm: "10px"
    md: "14px"
    lg: "20px"
    xl: "28px"
  shadow:
    glass_panel: "0 18px 48px rgba(71, 111, 180, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.82)"
    floating_chip: "0 10px 28px rgba(78, 128, 210, 0.18)"
    crystal_glow: "0 0 24px rgba(110, 220, 255, 0.30)"
layout:
  desktop_16_9:
    character_space: "65-70%"
    information_space: "30-35%"
    information_columns: ["chat_log", "topic_and_status", "worker_status", "daily_message"]
  mobile_portrait:
    character_space: "55-62%"
    information_space: "38-45%"
    information_behavior: "stack cards vertically while preserving chat input visibility"
motion:
  idle: "slow hologram breathing and very subtle crystal particles"
  search: "a pale cyan ring expands once"
  worker_running: "thin light traces travel across a white glass panel"
  complete: "small soft particles disperse upward"
  error: "soft amber shimmer, never harsh red alarm"
  notification: "glass card floats upward quietly"
accessibility:
  contrast_target: "WCAG AA for all persistent text"
  japanese_text: "must not overlap, clip, or require horizontal scrolling in fixed UI controls"
  reduced_motion: "disable particle and scan effects when reduced motion is requested"
---

# RenCrow IdleChat Design Guide

RenCrow IdleChat uses a bright sci-fi fantasy laboratory as its default visual world.
It should feel like a white AI lab, an observation room, and a gentle magical-engineering space where the characters are talking, thinking, and running work.

The visual target is not dark cyberpunk. Avoid hacker-room, combat bridge, horror lab, surveillance-room, or heavy mechanical background impressions.

## Core Image

The upper area is the character presence space. Mio and Shiro are the primary focus, with a white-to-pale-gray lab, soft cyan and lavender glow lines, translucent panels, holograms, crystals, and restrained magic-circle UI behind them.

The lower area is the information space. It uses white translucent glass cards with strong text readability. The UI should support conversation, topic tracking, worker progress, notifications, and short status summaries without competing with the characters.

## Adopt

- White laboratory walls and bright observation-room lighting.
- Translucent glass panels with pale blue borders.
- Floating holograms, circular magic-engineering UI, crystals, light particles, rune-like fine patterns.
- Soft floor reflections and gentle bloom.
- Pale cyan, white, soft lavender, and thin pale gold accents.
- Calm animations that become stronger only during search, worker execution, completion, error, and notification states.

## Avoid

- Full black cyberpunk backgrounds.
- Strong red or black warning UI.
- Excessive glitch effects.
- Dense mechanical panels that make the scene feel like a warship bridge.
- Horror-lab lighting or threatening experiment-room imagery.
- One-note blue palettes without white, lavender, and gold balance.

## Layout Rules

For a 16:9 stream view, reserve the upper 65-70% for characters and the lab world. Reserve the lower 30-35% for structured information.

For a mobile portrait view, reserve the upper 55-62% for the character and world, and the lower 38-45% for stacked information cards.

The chat input must remain visible and usable. IdleChat cards, worker panels, and TTS status elements must never overlap the input area.

## Component Rules

Cards use glass surfaces, pale borders, and compact spacing. They should feel light, not like heavy dark dashboards.

Buttons and chips should use icon-first controls where possible, with short labels only when the action is unclear without text.

State indicators should be small and calm. `LIVE`, worker progress, notification badges, and health states should read quickly without becoming visual alarms.

Errors should use amber shimmer and concise text. Red is reserved for truly destructive or system-failing conditions.

## Character Priority

Mio, Shiro, Aka, Ao, Gin, Kin, Kuro, and Midori should be readable as characters with distinct roles. UI panels must not cover faces or important character silhouettes in the primary scene.

Mio can lean softer, brighter, and more magical. Shiro can lean sharper, quieter, and more analytical. Both still belong to the same white sci-fi fantasy lab.

## Implementation Notes

The current Viewer may still contain dark cyberpunk tokens. When updating CSS, move gradually toward this guide:

1. Replace dark base backgrounds with bright lab gradients and translucent overlays.
2. Keep text contrast high with navy text on white glass.
3. Convert neon effects into pale cyan or lavender hologram effects.
4. Convert red error emphasis into amber unless the state is critical.
5. Verify desktop 16:9 and mobile portrait screenshots before accepting the change.
