# Faryen Admin Assets

Vite, TypeScript, and SCSS assets for the admin interface.

From `web/admin/assets`:

```sh
npm ci
npm run dev
```

`src/main.ts` and `src/style.scss` are manifest entries. The script also imports the stylesheet. During development, the Vite server serves this entry at `/src/main.ts`; server-rendered pages can load it with `/@vite/client` for hot reload.

Run `npm run build` to type-check and compile JavaScript, CSS, and imported assets directly into `dist/`. Output filenames include content hashes, and `.vite/manifest.json` maps source paths to those files. HTML is rendered by the backend.

The Go assets package exposes `BuiltAsset` and `BuiltCSS` as package-level aliases. `admin.RegisterHandlers(ctx, mux, builtFS)` loads the manifest from a filesystem rooted at `dist` and serves the compiled assets at `/assets/admin/`. See [the asset integration documentation](../../../docs/assets.md) for wiring and template usage.

Use `npm run preview` to serve the compiled assets locally.
