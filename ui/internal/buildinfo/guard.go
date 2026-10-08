//go:build production && (server || mcp)

package buildinfo

// A release build carries the production tag, and a test build carries server
// or mcp. Asking for both fails to compile here, so a test build cannot ship by
// a tag slipping into a release recipe
var _ = ReleaseBuildsMustNotCarryTheServerOrMcpTags
