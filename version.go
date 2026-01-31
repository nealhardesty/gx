package main

import "github.com/nealhardesty/gx/internal/version"

// Version is the current semantic version of the application.
// It re-exports version.Version for backward compatibility.
const Version = version.Version
