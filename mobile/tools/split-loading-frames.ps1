param(
  [Parameter(Mandatory = $true)]
  [string]$Source,
  [Parameter(Mandatory = $true)]
  [string]$OutputDirectory,
  [int]$Columns = 5,
  [int]$Rows = 2,
  [int]$FrameWidth = 288,
  [int]$FrameHeight = 720
)

$ErrorActionPreference = "Stop"
[System.IO.Directory]::CreateDirectory($OutputDirectory) | Out-Null
Add-Type -AssemblyName System.Drawing

$sheet = [System.Drawing.Bitmap]::new($Source)
$cellWidth = [int]($sheet.Width / $Columns)
$cellHeight = [int]($sheet.Height / $Rows)
$argb = [System.Drawing.Imaging.PixelFormat]::Format32bppArgb
try {
  for ($index = 0; $index -lt ($Columns * $Rows); $index++) {
    $column = $index % $Columns
    $row = [int][Math]::Floor($index / $Columns)
    $cell = [System.Drawing.Bitmap]::new($cellWidth, $cellHeight, $argb)
    $cellGraphics = [System.Drawing.Graphics]::FromImage($cell)
    try {
      $cellGraphics.Clear([System.Drawing.Color]::Transparent)
      $cellGraphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
      $cellGraphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
      $sourceRect = [System.Drawing.Rectangle]::new($column * $cellWidth, $row * $cellHeight, $cellWidth, $cellHeight)
      $targetRect = [System.Drawing.Rectangle]::new(0, 0, $cellWidth, $cellHeight)
      $cellGraphics.DrawImage($sheet, $targetRect, $sourceRect, [System.Drawing.GraphicsUnit]::Pixel)
    }
    finally {
      $cellGraphics.Dispose()
    }

    $frame = [System.Drawing.Bitmap]::new($FrameWidth, $FrameHeight, $argb)
    $frameGraphics = [System.Drawing.Graphics]::FromImage($frame)
    try {
      $frameGraphics.Clear([System.Drawing.Color]::Transparent)
      $frameGraphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
      $frameGraphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
      $frameGraphics.DrawImage($cell, [System.Drawing.Rectangle]::new(0, 0, $FrameWidth, $FrameHeight))
    }
    finally {
      $frameGraphics.Dispose()
      $cell.Dispose()
    }

    $outputPath = Join-Path $OutputDirectory ("character-{0:D2}.png" -f ($index + 1))
    $frame.Save($outputPath, [System.Drawing.Imaging.ImageFormat]::Png)
    $frame.Dispose()
  }
}
finally {
  $sheet.Dispose()
}

Get-ChildItem -LiteralPath $OutputDirectory -Filter "character-*.png" |
  Sort-Object Name |
  Select-Object Name, Length
