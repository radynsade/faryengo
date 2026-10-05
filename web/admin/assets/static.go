package assets

import "github.com/radynsade/faryengo/pkg/staticast"

const staticURLPrefix = URLPrefix + "static/"

var ErrStaticAssetNotFound = staticast.ErrAssetNotFound

var staticServer = staticast.New()

// StaticAsset resolves a path relative to static/ to its embedded asset URL.
var StaticAsset = staticServer.StaticAsset
