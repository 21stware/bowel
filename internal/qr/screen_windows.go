package qr

import (
	"os/exec"
	"path/filepath"
	"syscall"
)

// screenScript copies the whole virtual screen (every display) into a
// PNG, with .NET's own drawing classes: Windows has no command for it.
const screenScript = `Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$b = [System.Windows.Forms.SystemInformation]::VirtualScreen
$bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($b.Left, $b.Top, 0, 0, $bmp.Size)
$bmp.Save($env:BOWEL_QR_FILE, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()`

// capture takes one picture of all the displays together.
func capture(dir string) ([]string, error) {
	file := filepath.Join(dir, "screen.png")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", screenScript)
	cmd.Env = append(cmd.Environ(), "BOWEL_QR_FILE="+file)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return []string{file}, cmd.Run()
}
