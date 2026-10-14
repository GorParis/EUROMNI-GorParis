package frontend

import "embed"

//go:embed index.html euromni-catalog.js images/*.svg
var Assets embed.FS

//go:embed admin/*
var AdminAssets embed.FS
