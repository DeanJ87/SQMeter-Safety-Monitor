package web

import "embed"

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
//go:embed static/fonts/ibm-plex/*.woff2
var staticFS embed.FS
