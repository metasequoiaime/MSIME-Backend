package msimebackend

import (
	_ "embed"
	"strings"
)

// releaseVersion is embedded from the same source as image tags.
//
//go:embed VERSION
var releaseVersion string

// Version returns the version compiled into this server.
func Version() string { return strings.TrimSpace(releaseVersion) }
