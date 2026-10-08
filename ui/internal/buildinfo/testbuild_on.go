//go:build server || mcp

package buildinfo

// TestBuild is true in a build made for tests, which carries the server or the
// mcp tag. Such a build reads its settings from the environment and never ships
const TestBuild = true
