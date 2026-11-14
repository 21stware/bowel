package main

import "golang.org/x/sys/windows/registry"

// taskbarLight reports whether the Windows taskbar is light, which the
// system's "Windows mode" (Settings → Personalization → Colors) sets apart
// from the apps' mode. Without the value (Windows 10 before 1903) the
// taskbar is dark.
func taskbarLight() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v != 0
}
