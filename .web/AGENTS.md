# Gate Web Docs Agent Notes

These instructions apply to the VitePress documentation under `.web/docs`.

## Navigation

- Keep the top navigation limited to broad entry points. `Bedrock` and `Lite Mode` are acceptable header entries because they are primary user-facing Gate modes.
- Do not add lower-level runtime pages such as `GeyserLite` and `ViaLite` to the header. Put them in the Guide sidebar near the related user-facing feature.
- Do not use emoji in sidebar labels, top navigation labels, headings, or other structural navigation text. The docs should read like professional technical documentation.
- Keep sidebar labels short and scannable. Prefer direct nouns such as `Bedrock Support`, `ViaLite`, and `Configuration`.

## Content Shape

- Avoid duplicating setup guidance across Bedrock, GeyserLite, ViaLite, compatibility, and multi-version pages.
- Use `guide/bedrock.md` for operator-facing Bedrock setup.
- Use `/geyserlite/` to explain the managed Bedrock runtime, release policy, and relation to GeyserMC.
- Use `guide/compatibility.md` for server implementation compatibility. Its path and title should stay aligned around compatibility.
- Use `guide/multi-version.md` for user-facing Via-powered multi-version setup.
- Use `/vialite/` for the lower-level ViaLite runtime, topology, and release policy.

## Verification

- Run the VitePress build from `.web` before opening a docs PR:

  ```sh
  pnpm build
  ```

- When changing navigation, check the rendered desktop and mobile docs navigation before merging.
