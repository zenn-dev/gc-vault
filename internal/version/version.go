package version

// Version is set via -ldflags at build time.
// Defaults to a development tag when built without the flag.
var Version = "0.0.1-dev"
