package config

// Version is the git tag at build time; "dev" when built without ldflags.
// Inject via:
//
//	go build -ldflags "-X github.com/ocyss/tg115-follow/internal/config.Version=$(git describe --tags)"
var Version = "dev"
