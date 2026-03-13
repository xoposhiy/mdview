$ErrorActionPreference = "Stop"
$root = $PSScriptRoot

Add-Type -AssemblyName System.Drawing

# Inline C# for writing ICO binary format
Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Drawing.Imaging;
using System.IO;

public static class IcoWriter {
    public static void Save(Bitmap[] bitmaps, string path) {
        var pngs = new List<byte[]>();
        foreach (var bmp in bitmaps) {
            using (var ms = new MemoryStream()) {
                bmp.Save(ms, ImageFormat.Png);
                pngs.Add(ms.ToArray());
            }
        }
        int n = pngs.Count;
        int offset = 6 + n * 16;
        using (var fs = new FileStream(path, FileMode.Create))
        using (var bw = new BinaryWriter(fs)) {
            bw.Write((short)0); // reserved
            bw.Write((short)1); // type = icon
            bw.Write((short)n);
            for (int i = 0; i < n; i++) {
                int w = bitmaps[i].Width;
                int h = bitmaps[i].Height;
                bw.Write((byte)(w >= 256 ? 0 : w));
                bw.Write((byte)(h >= 256 ? 0 : h));
                bw.Write((byte)0);    // color count
                bw.Write((byte)0);    // reserved
                bw.Write((short)1);   // planes
                bw.Write((short)32);  // bit count
                bw.Write((int)pngs[i].Length);
                bw.Write((int)offset);
                offset += pngs[i].Length;
            }
            foreach (var png in pngs) bw.Write(png);
        }
    }
}
'@ -ReferencedAssemblies "System.Drawing"

function New-IconBitmap([int]$size) {
    $bmp = New-Object System.Drawing.Bitmap($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g   = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode      = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.TextRenderingHint  = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
    $g.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality

    # Rounded-square background: warm amber
    $bg   = [System.Drawing.Color]::FromArgb(255, 200, 145, 10)
    $bgBr = New-Object System.Drawing.SolidBrush($bg)
    $r    = [Math]::Max(2, [int]($size * 0.18))
    $path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $path.AddArc(0,          0,          $r*2, $r*2, 180, 90)
    $path.AddArc($size-$r*2, 0,          $r*2, $r*2, 270, 90)
    $path.AddArc($size-$r*2, $size-$r*2, $r*2, $r*2,   0, 90)
    $path.AddArc(0,          $size-$r*2, $r*2, $r*2,  90, 90)
    $path.CloseFigure()
    $g.FillPath($bgBr, $path)

    $cream = [System.Drawing.Color]::FromArgb(255, 255, 248, 215)
    $fgBr  = New-Object System.Drawing.SolidBrush($cream)

    if ($size -ge 24) {
        # Bold "M"
        $fontSize = [float]($size * 0.52)
        $font = New-Object System.Drawing.Font(
            "Segoe UI", $fontSize,
            [System.Drawing.FontStyle]::Bold,
            [System.Drawing.GraphicsUnit]::Pixel)
        $fmt = New-Object System.Drawing.StringFormat
        $fmt.Alignment     = [System.Drawing.StringAlignment]::Center
        $fmt.LineAlignment = [System.Drawing.StringAlignment]::Near
        $mRect = New-Object System.Drawing.RectangleF([float]0, [float]($size * 0.06), [float]$size, [float]($size * 0.60))
        $g.DrawString("M", $font, $fgBr, $mRect, $fmt)
        $font.Dispose()

        # Downward chevron (V shape)
        $cw  = [float]($size * 0.46)
        $ch  = [float]($size * 0.20)
        $cx  = [float]($size / 2)
        $cy  = [float]($size * 0.72)
        $pts = @(
            [System.Drawing.PointF]::new($cx - $cw/2, $cy),
            [System.Drawing.PointF]::new($cx,          $cy + $ch),
            [System.Drawing.PointF]::new($cx + $cw/2, $cy)
        )
        $penW = [Math]::Max(1.5, [float]($size * 0.095))
        $pen = New-Object System.Drawing.Pen($cream, $penW)
        $pen.LineJoin  = [System.Drawing.Drawing2D.LineJoin]::Round
        $pen.StartCap  = [System.Drawing.Drawing2D.LineCap]::Round
        $pen.EndCap    = [System.Drawing.Drawing2D.LineCap]::Round
        $g.DrawLines($pen, $pts)
        $pen.Dispose()
    }

    $g.Dispose(); $bgBr.Dispose(); $fgBr.Dispose()
    return $bmp
}

# Generate sizes
$sizes   = @(16, 24, 32, 48, 64, 128, 256)
$bitmaps = $sizes | ForEach-Object { New-IconBitmap $_ }

$icoPath = "$root\app.ico"
[IcoWriter]::Save($bitmaps, $icoPath)
$bitmaps | ForEach-Object { $_.Dispose() }
Write-Host "[icon] app.ico created ($($sizes -join ', ') px)"

# Install rsrc if missing
$rsrc = Get-Command rsrc -ErrorAction SilentlyContinue
if (-not $rsrc) {
    Write-Host "[icon] Installing rsrc..."
    & go install github.com/akavel/rsrc@latest
    if ($LASTEXITCODE -ne 0) { Write-Error "rsrc install failed"; exit 1 }
}

# Generate .syso
Write-Host "[icon] Generating rsrc_windows.syso..."
& rsrc -ico "$root\app.ico" -o "$root\src\rsrc_windows.syso"
if ($LASTEXITCODE -ne 0) { Write-Error "rsrc failed"; exit 1 }
Write-Host "[icon] Done. Run build.ps1 to rebuild with the icon."
