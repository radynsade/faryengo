# viteast

`pkg/viteast` reads a Vite build manifest, resolves source paths to the URLs of
their compiled, content-hashed files, and serves those files over HTTP. Use it
for any frontend built with Vite. For assets that keep their filenames, use
[staticast](staticast.md). How the admin panel builds, embeds, and serves its
frontend is described in [assets.md](../assets.md).

Import `github.com/radynsade/faryengo/pkg/viteast`.

## Overview

`viteast.New()` returns a `*Vite` that is both a URL resolver and an
`http.Handler`. `Load` reads a build and makes it active; until then, lookups
fail with `ErrNotLoaded` and HTTP requests receive 503.

| Member | Behavior |
| --- | --- |
| `New()` | Creates an instance and binds its `BuiltAsset` and `BuiltCSS` function fields. |
| `Load(ctx, built, baseURL)` | Reads and validates the manifest, then atomically replaces the active build. |
| `BuiltAsset(source)` | Returns the URL of the compiled file for a path relative to `src/`. |
| `BuiltCSS(source)` | Like `BuiltAsset`, but fails with `ErrNotCSS` unless the output is a `.css` file. |
| `ServeHTTP` | Serves files referenced by the manifest under the base URL's path. |

Always create instances with `New`; a zero `Vite` has nil function fields.

## Vite configuration

Enable the manifest and list every file the server needs to resolve as an
input. Stylesheets loaded with their own `<link>` tag need their own entry.

```ts
// vite.config.ts
export default defineConfig({
  base: "./",
  build: {
    outDir: "dist",
    manifest: true, // Writes dist/.vite/manifest.json.
    rollupOptions: {
      input: ["src/main.ts", "src/style.scss"],
    },
  },
});
```

A relative `base` keeps URLs inside compiled files working under whatever path
the Go server mounts the build at.

## Loading a build

Embed the build directory with `all:` so the hidden `.vite` directory is
included, then pass a filesystem rooted at that directory:

```go
//go:embed all:dist
var embeddedFiles embed.FS

var server = viteast.New()

// Package-level aliases for templates.
var (
	BuiltAsset = server.BuiltAsset
	BuiltCSS   = server.BuiltCSS
)

func init() {
	built, err := fs.Sub(embeddedFiles, "dist")
	if err != nil {
		panic(fmt.Errorf("open embedded assets: %w", err))
	}

	if err := server.Load(context.Background(), built, "/assets/app/"); err != nil {
		panic(fmt.Errorf("load embedded assets: %w", err))
	}
}
```

Loading an embedded build during initialization makes a broken build fail at
startup rather than on the first page render.

`Load` reads `.vite/manifest.json` and validates the whole build before
activating it. It returns `ErrInvalidManifest` when the manifest is malformed or
empty, when a source maps to two different outputs, when an entry imports a
chunk missing from the manifest, or when an output path is invalid, hidden, or
not a regular file. A referenced file that does not exist returns the
underlying `fs` error.

`baseURL` is either a root-relative prefix (`/assets/app/`) or an absolute
HTTP(S) URL (`https://cdn.example.com/app/`). A trailing slash is added if
missing. Prefixes with a query, fragment, user info, backslashes, `//`, or
unclean segments return `ErrInvalidBaseURL`. A nil filesystem returns
`ErrNilFilesystem`, and a cancelled context returns its error.

A failed `Load` leaves the previous build active. A successful `Load` replaces
it atomically, so concurrent requests and existing aliases keep working during a
reload.

## Resolving URLs

Source paths are relative to `src/`: `BuiltAsset("main.ts")` looks up the
manifest entry `src/main.ts`.

```go
scriptURL, err := server.BuiltAsset("main.ts")
// "/assets/app/assets/main-B4x9Kq2c.js"

styleURL, err := server.BuiltCSS("style.scss")
// "/assets/app/assets/style-Dp3sV1aZ.css"

_, err = server.BuiltCSS("main.ts")
// errors.Is(err, viteast.ErrNotCSS)
```

Unknown sources return `ErrAssetNotFound`. `BuiltCSS` checks the entry's own
output file; it does not return the CSS that Vite extracts from a JavaScript
entry, which is why stylesheets need their own input.

Both helpers return `(string, error)`, which templ accepts in URL attributes:

```templ
<link rel="stylesheet" href={ assets.BuiltCSS("style.scss") } />
<script type="module" src={ assets.BuiltAsset("main.ts") }></script>
```

## Serving files

Mount the instance at the base URL's path, outside any routes with wildcard
prefixes such as `/{language}/`:

```go
mux := http.NewServeMux()
mux.Handle("/assets/app/", server)
```

The handler accepts only `GET` and `HEAD`; other methods receive 405 with an
`Allow: GET, HEAD` header. It serves the files the manifest references (entry
outputs, their CSS, and their assets) using `http.FileServerFS`, including
range and conditional requests, and sets `X-Content-Type-Options: nosniff`.
The manifest itself, unreferenced files, and directory listings receive 404.

## Errors

| Error | Returned when |
| --- | --- |
| `ErrNotLoaded` | A helper is called before a successful `Load`. |
| `ErrAssetNotFound` | The source path has no manifest entry. |
| `ErrNotCSS` | `BuiltCSS` resolves to a non-CSS file. |
| `ErrInvalidManifest` | The manifest or a file it references fails validation. |
| `ErrInvalidBaseURL` | `Load` receives an unusable base URL. |
| `ErrNilFilesystem` | `Load` receives a nil filesystem. |

Errors are wrapped with context; compare them with `errors.Is`.
