# Datastar

`datastar.js` is the official Datastar v1.0.4 bundle from
https://github.com/starfederation/datastar/blob/v1.0.4/bundles/datastar.js.
Only its source map reference is removed. The MIT license is included alongside
it. Vite bundles this file into the embedded admin assets, with no runtime CDN
request. `datastar.d.ts` describes the action APIs used by admin navigation.

When upgrading, replace the bundle from a pinned official release and verify
navigation, HTML fragment handling, and browser history against that release.
