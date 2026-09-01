package platform

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func OpenURL(url string) tea.Cmd {
	return func() tea.Msg {
		if !trustedURL(url) {
			return fmt.Errorf("refusing to open untrusted URL")
		}
		var name string
		var args []string
		switch runtime.GOOS {
		case "darwin":
			name, args = "open", []string{url}
		case "windows":
			name, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
		default:
			name, args = "xdg-open", []string{url}
			if _, err := exec.LookPath(name); err != nil {
				name, args = "gio", []string{"open", url}
			}
		}
		return exec.Command(name, args...).Start()
	}
}

func trustedURL(value string) bool {
	for _, prefix := range []string{
		"https://delbysoft.com",
		"https://dev.twitch.tv/console/apps",
		"https://console.cloud.google.com/apis/credentials",
		"https://console.cloud.google.com/apis/library/youtube.googleapis.com",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
func EditorCommand(path, configured string) (*exec.Cmd, error) {
	command := strings.TrimSpace(configured)
	if command == "" {
		command = strings.TrimSpace(os.Getenv("VISUAL"))
	}
	if command == "" {
		command = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if command == "" {
		if runtime.GOOS == "windows" {
			command = "notepad"
		} else {
			for _, candidate := range []string{"nano", "vim", "vi"} {
				if _, err := exec.LookPath(candidate); err == nil {
					command = candidate
					break
				}
			}
		}
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("no text editor found")
	}
	return exec.Command(parts[0], append(parts[1:], path)...), nil
}
