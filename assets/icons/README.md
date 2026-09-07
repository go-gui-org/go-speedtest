# Application icons

All files here come from `assets/logo.svg`. That SVG holds one 1024x1024 PNG.
The PNG has a white page background, a drop shadow, and a generator watermark in
the bottom-right corner. The icons are the artwork tile only: the tile occupies
pixels 87 to 936 on both axes (850x850), and a round-rectangle alpha mask with a
191px corner radius removes the corners, the shadow, and the watermark.

## Files

| File           | Use                                                     |
| -------------- | ------------------------------------------------------- |
| `icon.icns`    | macOS `.app` bundle (`buildapp -platform darwin -icon`) |
| `icon.ico`     | Windows executable (`buildapp -platform windows -icon`) |
| `icon-256.png` | Linux hicolor theme (`buildapp -platform linux -icon`)  |
| `icon-<N>.png` | Single sizes: 16, 32, 48, 64, 128, 256, 512, 1024       |

`assets/icon.png` is a copy of `icon-256.png`. It is the default icon for
`buildapp` when no platform-specific file is given.

## How to make the icons again

The commands need ImageMagick (`magick`) and the macOS `iconutil` tool.

1. Get the PNG out of the SVG:

   ```fish
   python3 -c '
   import base64, re, sys
   s = open("assets/logo.svg").read()
   m = re.search(r"base64,([A-Za-z0-9+/=\s]+)\"", s)
   open(sys.argv[1], "wb").write(base64.b64decode(re.sub(r"\s", "", m.group(1))))
   ' /tmp/master.png
   ```

2. Cut out the tile and mask the corners:

   ```fish
   magick /tmp/master.png -crop 850x850+87+87 +repage /tmp/tile_raw.png
   magick -size 850x850 xc:none -fill white \
     -draw "roundrectangle 0,0,849,849,191,191" /tmp/mask.png
   magick /tmp/tile_raw.png /tmp/mask.png -alpha off \
     -compose CopyOpacity -composite /tmp/tile.png
   ```

3. Make each size:

   ```fish
   magick /tmp/tile.png -filter Lanczos -resize 1024x1024 -strip \
     PNG32:assets/icons/icon-1024.png
   for s in 512 256 128 64 48 32 16
     magick assets/icons/icon-1024.png -filter Lanczos -resize {$s}x{$s} \
       -strip PNG32:assets/icons/icon-$s.png
   end
   ```

4. Make the `.icns` and the `.ico`. The iconset names are the names Apple needs;
   `iconutil` refuses other names.

   ```fish
   mkdir -p /tmp/icon.iconset
   cp assets/icons/icon-16.png   /tmp/icon.iconset/icon_16x16.png
   cp assets/icons/icon-32.png   /tmp/icon.iconset/icon_16x16@2x.png
   cp assets/icons/icon-32.png   /tmp/icon.iconset/icon_32x32.png
   cp assets/icons/icon-64.png   /tmp/icon.iconset/icon_32x32@2x.png
   cp assets/icons/icon-128.png  /tmp/icon.iconset/icon_128x128.png
   cp assets/icons/icon-256.png  /tmp/icon.iconset/icon_128x128@2x.png
   cp assets/icons/icon-256.png  /tmp/icon.iconset/icon_256x256.png
   cp assets/icons/icon-512.png  /tmp/icon.iconset/icon_256x256@2x.png
   cp assets/icons/icon-512.png  /tmp/icon.iconset/icon_512x512.png
   cp assets/icons/icon-1024.png /tmp/icon.iconset/icon_512x512@2x.png
   iconutil -c icns /tmp/icon.iconset -o assets/icons/icon.icns

   magick assets/icons/icon-256.png \
     -define icon:auto-resize=256,128,64,48,32,16 assets/icons/icon.ico
   ```
