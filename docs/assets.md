# Built web assets

Each frontend produces a Vite build in its own `assets/dist` directory. Vite emits `.vite/manifest.json` to map source paths to compiled filenames. The admin assets package embeds the complete build, including the hidden `.vite` manifest, into the Go binary with `//go:embed all:dist`. It loads and validates the embedded manifest during package initialization. `web/vite` resolves URLs and implements `http.Handler` to serve the embedded files.

`make build` compiles the admin assets, generates templ code, runs the Go checks, and builds the binaries. `make check` also builds assets before checking Go code. npm dependencies are installed with `npm ci` when missing or when the package files change.

For direct Go commands, build the frontend first:

```sh
cd web/admin/assets
npm ci
npm run build
```

Wire the admin routes in the Go server:

```go
adminHandler, err := admin.NewHandler(authenticationService, signInLimiter, secureCookies)
if err != nil {
    return fmt.Errorf("configure admin: %w", err)
}

mux := http.NewServeMux()
if err := adminHandler.RegisterHandlers(mux); err != nil {
    return fmt.Errorf("configure admin: %w", err)
}
```

The handler receives the application authentication service, sign-in rate limiter,
and cookie security setting. Registration only mounts routes, so it takes no
context or filesystem argument. The binary serves assets independently of its
working directory and does not need `dist` on disk at runtime. Rebuild the Go
binary after changing frontend assets.

The admin assets are served at `/assets/admin/`. The template imports `web/admin/assets` and calls its package-level aliases:

```templ
<link rel="stylesheet" href={ assets.BuiltCSS("style.scss") } />
<script type="module" src={ assets.BuiltAsset("main.ts") }></script>
```

Both helpers return `(string, error)`, which templ accepts in URL attributes. Paths
are relative to `src/`: `main.ts` looks up `src/main.ts` in the manifest, and
`styles/page.scss` looks up `src/styles/page.scss`. They return URLs containing
the actual compiled filenames. Unknown source paths return
`vite.ErrAssetNotFound`. `BuiltCSS` rejects non-CSS output with `vite.ErrNotCSS`.

A stylesheet must have its own manifest entry to be resolved by source path. The admin Vite configuration includes both `src/main.ts` and `src/style.scss` as inputs. Its relative `base` lets references inside the compiled assets work under the Go server's mount path.

Place files that should retain their filenames, such as fonts and images, in
`web/admin/assets/static/`. This directory is embedded directly in the Go binary
and served at `/assets/admin/static/` by the same handler registration. Rebuild
the binary after adding or changing a static file.

`assets.StaticAsset("images/logo.svg")` returns `(string, error)` for a path
relative to `static/`, suitable for templ URL attributes. It checks that the
embedded file exists and escapes the URL. Missing files, directories, hidden
paths, and invalid paths return `assets.ErrStaticAssetNotFound`. Static serving
supports GET, HEAD, and range requests and does not expose directory listings.

For another frontend, create a private `vite.New()` instance in its assets package and expose `BuiltAsset` and `BuiltCSS` as aliases to that instance's function fields. Embed its build directory, obtain a filesystem rooted at `dist` with `fs.Sub`, and initialize the instance with `Load(ctx, builtFS, baseURL)` before mounting it on a ServeMux. `baseURL` can be a root-relative prefix or an absolute HTTP(S) URL; mount it at the URL's path. Choose an asset prefix outside routes with language wildcards.

The generic Vite instance returns `vite.ErrNotLoaded` before initialization. A successful `Load` replaces the manifest snapshot atomically, so existing aliases keep working. A failed reload keeps the previous snapshot. The HTTP handler supports GET and HEAD, including range requests. It serves manifest-referenced assets and does not expose the manifest, unlisted files, or directory listings.
