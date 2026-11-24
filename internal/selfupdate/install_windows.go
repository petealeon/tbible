//go:build windows

package selfupdate

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Install stages the new executable and starts a hidden helper that replaces
// the running .exe after this process exits. Windows cannot overwrite an
// executable image while it is running.
func Install(staged, target string) error {
	newPath := target + ".new"
	scriptPath := target + ".update.ps1"
	_ = os.Remove(newPath)
	_ = os.Remove(scriptPath)

	if err := copyFile(staged, newPath); err != nil {
		return err
	}
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$targetPid = %d
$deadline = (Get-Date).AddMinutes(5)
while ((Get-Process -Id $targetPid -ErrorAction SilentlyContinue) -and ((Get-Date) -lt $deadline)) {
  Start-Sleep -Milliseconds 200
}
$target = %s
$new = %s
$script = %s
try {
  if (-not (Test-Path -LiteralPath $new)) { throw 'staged update is missing' }
  Move-Item -LiteralPath $new -Destination $target -Force
} finally {
  Remove-Item -LiteralPath $new -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $script -Force -ErrorAction SilentlyContinue
}
`, os.Getpid(), psQuote(target), psQuote(newPath), psQuote(scriptPath))
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		_ = os.Remove(newPath)
		return err
	}

	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		_ = os.Remove(newPath)
		_ = os.Remove(scriptPath)
		return fmt.Errorf("start update helper: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
