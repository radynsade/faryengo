# staticast

`pkg/staticast` serves files from a static asset directory and resolves their
public URLs. Use it for assets that keep their filenames, such as fonts, images,
and favicons. For compiled Vite output with hashed filenames, use
[viteast](viteast.md). How the admin panel wires both packages is described in
[assets.md](../assets.md).

Import `github.com/radynsade/faryengo/pkg/staticast`.

## Overview

`staticast.New()` returns a `*Static` that is both a URL resolver and an
`http.Handler`. `Load` indexes a filesystem and makes it active; until then,
lookups fail with `ErrNotLoaded` and HTTP requests receive 503.

| Member | Behavior |
| --- | --- |
| `New()` | Creates an instance and binds its `StaticAsset` function field. |
| `Load(ctx, files, baseURL)` | Indexes the regular files in `files` and atomically replaces the active snapshot. |
| `StaticAsset(source)` | Returns the escaped URL of an indexed file, given its path relative to the filesystem root. |
| `ServeHTTP` | Serves indexed files under the base URL's path. |

Always create instances with `New`; a zero `Static` has a nil `StaticAsset`.

## Loading assets

```go
//go:embed static
var embeddedFiles embed.FS

var server = staticast.New()

// StaticAsset is exposed as a package-level alias for templates.
var StaticAsset = server.StaticAsset

func Load(ctx context.Context) error {
	files, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		return fmt.Errorf("open static assets: %w", err)
	}

	return server.Load(ctx, files, "/assets/static/")
}
```

`files` must be rooted at the static directory. Its files must implement
`io.Seeker`, as `http.FileServerFS` also requires; `embed.FS` and `os.DirFS`
both qualify. The filesystem must not change while loaded. To update assets,
load a new filesystem.

Indexing includes only regular files. Directories, symlinks, invalid paths,
and any path with a segment starting with `.` (such as `.well-known/` or
`.DS_Store`) are skipped. An empty directory is valid. A root that is not a
directory returns `fs.ErrInvalid`.

`baseURL` is either a root-relative prefix (`/assets/static/`) or an absolute
HTTP(S) URL (`https://cdn.example.com/static/`). A trailing slash is added if
missing. Prefixes containing a query, fragment, user info, backslashes, `//`, or
unclean segments such as `..` return `ErrInvalidBaseURL`. A nil filesystem
returns `ErrNilFilesystem`, and a cancelled context returns its error.

A failed or cancelled `Load` leaves the previous snapshot active. A successful
`Load` replaces it atomically, so concurrent requests and existing
`StaticAsset` aliases see either the old index or the new one, never a mix.

## Resolving URLs

```go
logoURL, err := server.StaticAsset("images/logo.svg")
// "/assets/static/images/logo.svg"

fontURL, err := server.StaticAsset("fonts/Inter Regular.woff2")
// "/assets/static/fonts/Inter%20Regular.woff2"
```

Paths are relative to the filesystem root and must name an indexed file.
Unknown files, directories, and hidden paths return `ErrAssetNotFound`. With an
absolute base URL, the result is absolute too.

The `(string, error)` signature fits templ URL attributes directly:

```templ
<link rel="icon" href={ assets.StaticAsset("favicon.svg") } />
<img src={ assets.StaticAsset("images/logo.svg") } alt="" />
```

## Serving files

Mount the instance at the base URL's path:

```go
mux := http.NewServeMux()
mux.Handle("/assets/static/", server)
```

The handler accepts only `GET` and `HEAD`; other methods receive 405 with an
`Allow: GET, HEAD` header. It serves only indexed files, with conditional and
range request support from `http.ServeContent`, and sets
`X-Content-Type-Options: nosniff`. Unindexed paths and paths outside the
prefix receive 404, and directories are never listed. A request for
`index.html` serves that file rather than redirecting.

For an absolute base URL, the handler matches on the URL's path, which suits a
CDN that pulls from the application server.

## Errors

| Error | Returned when |
| --- | --- |
| `ErrNotLoaded` | `StaticAsset` is called before a successful `Load`. |
| `ErrAssetNotFound` | The requested file is not in the index. |
| `ErrInvalidBaseURL` | `Load` receives an unusable base URL. |
| `ErrNilFilesystem` | `Load` receives a nil filesystem. |

Errors are wrapped with context; compare them with `errors.Is`.
