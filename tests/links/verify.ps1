# Drives mdview through the link/history flow and screenshots each step.
# Tab x2 reaches the first document link (the hidden back button is not
# focusable; the settings gear is the first tab stop), Enter activates it.
$ErrorActionPreference = "Stop"

$root      = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$mdviewExe = "$PSScriptRoot\..\..\mdview.exe"
$testFile  = "$PSScriptRoot\a.md"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public class WinCap2 {
    public delegate bool EnumDelegate(IntPtr hwnd, IntPtr lParam);
    [DllImport("user32.dll")] public static extern bool EnumWindows(EnumDelegate cb, IntPtr lp);
    [DllImport("user32.dll")] public static extern int  GetWindowText(IntPtr hwnd, StringBuilder sb, int max);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hwnd, out RECT r);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }
}
"@

# Default settle is generous: the page embeds ~2 MB of MathJax and a navigation
# needs time to repaint. Toast assertions pass a shorter wait so the shot lands
# inside the toast's 2.5 s lifetime.
function Send-Key([byte]$vk, [int]$settle = 2500) {
    [WinCap2]::keybd_event($vk, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 60
    [WinCap2]::keybd_event($vk, 0, 2, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds $settle
}

$proc = Start-Process -FilePath $mdviewExe -ArgumentList $testFile -PassThru
Start-Sleep -Milliseconds 3500

$hwnd = [IntPtr]::Zero
$cb = [WinCap2+EnumDelegate]{
    param($h, $lp)
    if ([WinCap2]::IsWindowVisible($h)) {
        $sb = New-Object System.Text.StringBuilder(512)
        [WinCap2]::GetWindowText($h, $sb, 512) | Out-Null
        if ($sb.ToString() -like "*mdview*") { $script:hwnd = $h; return $false }
    }
    return $true
}
[WinCap2]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null
if ($hwnd -eq [IntPtr]::Zero) {
    $proc | Stop-Process -Force -ErrorAction SilentlyContinue
    throw "mdview window not found"
}
[WinCap2]::SetForegroundWindow($hwnd)
Start-Sleep -Milliseconds 800

function Save-Shot([string]$name) {
    $r = New-Object WinCap2+RECT
    [WinCap2]::GetWindowRect($hwnd, [ref]$r)
    $w = $r.Right - $r.Left; $h = $r.Bottom - $r.Top
    $bmp = New-Object System.Drawing.Bitmap($w, $h)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $h)))
    $bmp.Save("$PSScriptRoot\$name.png", [System.Drawing.Imaging.ImageFormat]::Png)
    $g.Dispose(); $bmp.Dispose()
    Write-Host "saved $name.png"
}

Save-Shot "1-initial"

Send-Key 0x09          # Tab -> settings gear
Send-Key 0x09          # Tab -> "document B" link
Send-Key 0x0D          # Enter -> activate link
Save-Shot "2-followed-link"

Send-Key 0x08          # Backspace -> back
Save-Shot "3-after-back"

# Back on a.md with focus reset by the reload. Tab order is:
# settings gear, document B, Anthropic, missing file, in-page anchor.
Send-Key 0x09; Send-Key 0x09; Send-Key 0x09; Send-Key 0x09   # -> "missing file"
Send-Key 0x0D 400                                            # Enter -> toast, no navigation
Save-Shot "4-missing-file-toast"

# No navigation happened, so focus is still on "missing file"; one Tab reaches
# the in-page anchor.
Send-Key 0x09
Send-Key 0x0D          # Enter -> should scroll to "Section two", not navigate
Save-Shot "5-in-page-anchor"

# Finally the headline case: an external link must leave mdview untouched and
# hand off to the default browser. Reload (F5) resets focus to the document.
Send-Key 0x74          # F5
Send-Key 0x09; Send-Key 0x09; Send-Key 0x09                  # -> "Anthropic"
Send-Key 0x0D
Save-Shot "6-after-external-link"

$proc | Stop-Process -Force
Write-Host "done"
