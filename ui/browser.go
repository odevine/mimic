package main

import (
	"os/exec"
	"runtime"
)

// openBrowser opens url in the user's default browser. It shells out per OS, all
// via os/exec, so nothing here reintroduces cgo
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
