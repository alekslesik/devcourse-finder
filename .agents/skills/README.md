# Frontend design skills

Project-local skills from the three repositories requested by the maintainer.
Codex discovers each `.agents/skills/<name>/SKILL.md` in a supported client;
restart/reopen the session if its skill catalog was loaded before installation.
In clients without automatic discovery, read the matching file directly as
specified in the root `AGENTS.md`. No application dependency installation is needed.

All skill folders include their companion references and upstream license.
`sources.json` pins source commits, original paths and SHA-256 checksums for every
installed file. These snapshots do not update automatically. To update, fetch the
chosen upstream revision, replace its listed folders (Impeccable uses its compiled
`.agents/skills` distribution), retain licenses, refresh the manifest and review
changes in a PR. Do not edit upstream guidance silently.

## Selection

Use relevant guidance, not all skills at once. Existing product behavior, the
maintainer's request, this repository's workflow and its approved liquid glass
identity take precedence over generic design prescriptions. Some collections
include alternate aesthetics, image generators, native app or library-specific
skills; use those only for matching work and when the required tools are available.

Impeccable provides design planning, critique, accessibility and polishing.
Taste Skill provides visual direction and redesign guidance. Emil Kowalski's
skills focus on motion, touch behavior, UI robustness and design engineering.

## Impeccable engine

The official Codex distribution includes a launcher and version-pinned engine
metadata. Its first engine command may download the matching platform binary and
verify the release SHA-256 sidecar. Keep that cache outside Git. In a sandbox with
no home write access, use a writable cache, for example:

```sh
IMPECCABLE_HOME=/tmp/devcourse-impeccable .agents/skills/impeccable/scripts/impeccable engine-probe
```

The launcher can also use `IMPECCABLE_BIN` for an existing engine. Follow its
SKILL.md fallback when the engine cannot run. This skill-only installation does
not add automatic edit hooks; hooks can be installed separately using the upstream
installer if explicitly requested.

## Installed skills

| Skill | Source | Instructions |
| --- | --- | --- |
| `impeccable` | pbakaus/impeccable | [impeccable](./impeccable/SKILL.md) |
| `brandkit` | Leonxlnx/taste-skill | [brandkit](./brandkit/SKILL.md) |
| `industrial-brutalist-ui` | Leonxlnx/taste-skill | [industrial-brutalist-ui](./industrial-brutalist-ui/SKILL.md) |
| `gpt-taste` | Leonxlnx/taste-skill | [gpt-taste](./gpt-taste/SKILL.md) |
| `image-to-code` | Leonxlnx/taste-skill | [image-to-code](./image-to-code/SKILL.md) |
| `imagegen-frontend-mobile` | Leonxlnx/taste-skill | [imagegen-frontend-mobile](./imagegen-frontend-mobile/SKILL.md) |
| `imagegen-frontend-web` | Leonxlnx/taste-skill | [imagegen-frontend-web](./imagegen-frontend-web/SKILL.md) |
| `minimalist-ui` | Leonxlnx/taste-skill | [minimalist-ui](./minimalist-ui/SKILL.md) |
| `full-output-enforcement` | Leonxlnx/taste-skill | [full-output-enforcement](./full-output-enforcement/SKILL.md) |
| `redesign-existing-projects` | Leonxlnx/taste-skill | [redesign-existing-projects](./redesign-existing-projects/SKILL.md) |
| `high-end-visual-design` | Leonxlnx/taste-skill | [high-end-visual-design](./high-end-visual-design/SKILL.md) |
| `stitch-design-taste` | Leonxlnx/taste-skill | [stitch-design-taste](./stitch-design-taste/SKILL.md) |
| `design-taste-frontend` | Leonxlnx/taste-skill | [design-taste-frontend](./design-taste-frontend/SKILL.md) |
| `design-taste-frontend-v1` | Leonxlnx/taste-skill | [design-taste-frontend-v1](./design-taste-frontend-v1/SKILL.md) |
| `animate` | emilkowalski/skills | [animate](./animate/SKILL.md) |
| `animate-expo` | emilkowalski/skills | [animate-expo](./animate-expo/SKILL.md) |
| `animation-performance` | emilkowalski/skills | [animation-performance](./animation-performance/SKILL.md) |
| `animation-vocabulary` | emilkowalski/skills | [animation-vocabulary](./animation-vocabulary/SKILL.md) |
| `apple-design` | emilkowalski/skills | [apple-design](./apple-design/SKILL.md) |
| `ask-sonner` | emilkowalski/skills | [ask-sonner](./ask-sonner/SKILL.md) |
| `break-ui` | emilkowalski/skills | [break-ui](./break-ui/SKILL.md) |
| `emil-design-eng` | emilkowalski/skills | [emil-design-eng](./emil-design-eng/SKILL.md) |
| `find-animation-opportunities` | emilkowalski/skills | [find-animation-opportunities](./find-animation-opportunities/SKILL.md) |
| `improve-animations` | emilkowalski/skills | [improve-animations](./improve-animations/SKILL.md) |
| `mobile-native` | emilkowalski/skills | [mobile-native](./mobile-native/SKILL.md) |
| `pick-ui-library` | emilkowalski/skills | [pick-ui-library](./pick-ui-library/SKILL.md) |
| `prototype` | emilkowalski/skills | [prototype](./prototype/SKILL.md) |
| `review-animations` | emilkowalski/skills | [review-animations](./review-animations/SKILL.md) |
| `write-swift` | emilkowalski/skills | [write-swift](./write-swift/SKILL.md) |
