package main

import "embed"

// templatesFS holds every skeleton file stamped by `beach new`. Files ending in
// ".tmpl" are rendered through text/template with the app's data; all others are
// copied byte-for-byte.
//
//go:embed all:templates
var templatesFS embed.FS
