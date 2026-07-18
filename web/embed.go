package web

import "embed"

//go:embed templates/*.html assets/*
var Files embed.FS
