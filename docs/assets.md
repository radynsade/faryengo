# Built web assets

Each frontend produces a Vite build in its own `assets/dist` directory. Vite emits `.vite/manifest.json` to map source paths to compiled filenames. `web/vite` reads that manifest, checks its referenced files, resolves URLs, and implements `http.Handler` to serve the build.

Build the admin assets first:

```sh
cd web/admin/assets
npm ci
npm run build
```

Wire the admin routes in the Go server using a filesystem rooted at `dist`:

```go
mux := http.NewServeMux()
if err := admin.RegisterHandlers(ctx, mux, os.DirFS("web/admin/assets/dist")); err != nil {
    return fmt.Errorf("configure admin: %w", err)
}
```

The admin assets are served at `/assets/admin/`. The template imports `web/admin/assets` and calls its package-level aliases:

```templ
<link rel="stylesheet" href={ assets.BuiltCSS("src/style.scss") } />
<script type="module" src={ assets.BuiltAsset("src/main.ts") }></script>
```

Both helpers return `(string, error)`, which templ accepts in URL attributes. They resolve the source path to a URL containing the actual compiled filename. Unknown source paths return `vite.ErrAssetNotFound`. Calls before the build is loaded return `vite.ErrNotLoaded`, and `BuiltCSS` rejects non-CSS output with `vite.ErrNotCSS`.

A stylesheet must have its own manifest entry to be resolved by source path. The admin Vite configuration includes both `src/main.ts` and `src/style.scss` as inputs. Its relative `base` lets references inside the compiled assets work under the Go server's mount path.

For another frontend, create a private `vite.New()` instance in its assets package and expose `BuiltAsset` and `BuiltCSS` as aliases to that instance's function fields. Initialize it with `Load(ctx, builtFS, baseURL)`, then mount the instance on a ServeMux. `baseURL` can be a root-relative prefix or an absolute HTTP(S) URL; mount it at the URL's path. Choose an asset prefix outside routes with language wildcards.

A successful `Load` replaces the manifest snapshot atomically, so existing aliases keep working. A failed reload keeps the previous snapshot. The HTTP handler supports GET and HEAD, including range requests. It serves manifest-referenced assets and does not expose the manifest, unlisted files, or directory listings.
