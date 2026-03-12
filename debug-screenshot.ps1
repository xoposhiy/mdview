$ErrorActionPreference = "Stop"

$mdviewExe = "$PSScriptRoot\mdview.exe"
$testFile  = "$PSScriptRoot\specs\requirements.md"
$outFile   = "$PSScriptRoot\debug.png"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public class WinCapture {
    public delegate bool EnumDelegate(IntPtr hwnd, IntPtr lParam);
    [DllImport("user32.dll")] public static extern bool EnumWindows(EnumDelegate cb, IntPtr lp);
    [DllImport("user32.dll")] public static extern int  GetWindowText(IntPtr hwnd, StringBuilder sb, int max);
    [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hwnd, out RECT r);
    [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hwnd);
    [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hwnd, int cmd);
    [DllImport("user32.dll")] public static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left, Top, Right, Bottom; }
}
"@

# Minimize all windows (Win+D)
[WinCapture]::keybd_event(0x5B, 0, 0, [UIntPtr]::Zero)   # Win down
[WinCapture]::keybd_event(0x44, 0, 0, [UIntPtr]::Zero)   # D down
Start-Sleep -Milliseconds 100
[WinCapture]::keybd_event(0x44, 0, 2, [UIntPtr]::Zero)   # D up
[WinCapture]::keybd_event(0x5B, 0, 2, [UIntPtr]::Zero)   # Win up
Start-Sleep -Milliseconds 500

Write-Host "Starting mdview..."
$proc = Start-Process -FilePath $mdviewExe -ArgumentList $testFile -PassThru
Start-Sleep -Milliseconds 3000

# Find window
$hwnd = [IntPtr]::Zero
$cb = [WinCapture+EnumDelegate]{
    param($h, $lp)
    if ([WinCapture]::IsWindowVisible($h)) {
        $sb = New-Object System.Text.StringBuilder(512)
        [WinCapture]::GetWindowText($h, $sb, 512) | Out-Null
        if ($sb.ToString() -like "*mdview*") { $script:hwnd = $h; return $false }
    }
    return $true
}
[WinCapture]::EnumWindows($cb, [IntPtr]::Zero) | Out-Null

if ($hwnd -eq [IntPtr]::Zero) {
    $proc | Stop-Process -Force -ErrorAction SilentlyContinue
    throw "mdview window not found"
}

# Maximize and bring to front
[WinCapture]::ShowWindow($hwnd, 3)        # SW_MAXIMIZE
[WinCapture]::SetForegroundWindow($hwnd)
Start-Sleep -Milliseconds 1000

$r = New-Object WinCapture+RECT
[WinCapture]::GetWindowRect($hwnd, [ref]$r)
$w = $r.Right - $r.Left
$h = $r.Bottom - $r.Top

$bmp = New-Object System.Drawing.Bitmap($w, $h)
$g   = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.Left, $r.Top, 0, 0, (New-Object System.Drawing.Size($w, $h)))
$bmp.Save($outFile, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()

$proc | Stop-Process -Force
Write-Host "Saved: $outFile  ($w x $h)"
