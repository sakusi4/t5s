# Records docs/demo.gif with whatever t5s is on PATH. Start the sample
# services first:
#
#   docker compose -f hack/compose.yaml up -d
#   vhs hack/demo.tape
#
# The take shares dashboard with one address and opens the access log
# while visitors arrive. The GIF in the repository was recorded with a
# demo build whose tunnel and own address are fake, so no real URL or
# address is in it; visitors were curl requests carrying CF-Connecting-IP.
#
# VHS 0.12 cannot encode with ffmpeg 9. Until that is fixed, change the
# Output to a directory to get PNG frames and encode them by hand:
#
#   ffmpeg -framerate 50 -i frames/frame-text-%05d.png \
#     -framerate 50 -i frames/frame-cursor-%05d.png \
#     -filter_complex "[0][1]overlay,fps=20,split[a][b];[a]palettegen[p];[b][p]paletteuse" \
#     -loop 0 docs/demo.gif

Output docs/demo.gif

Set Shell "bash"
Set FontSize 15
Set Width 1120
Set Height 420
Set Padding 16
Set TypingSpeed 60ms
Set Theme { "background": "#000000", "foreground": "#ffffff", "cursor": "#A78BFA", "selection": "#5B21B6" }

Hide
Type "clear"
Enter
Show

Type "t5s"
Sleep 500ms
Enter
Sleep 2500ms

Type "j"
Sleep 400ms
Type "j"
Sleep 400ms
Type "k"
Sleep 300ms
Type "k"
Sleep 700ms

Type "s"
Sleep 1s
Type "198.51.100.23"
Sleep 800ms
Enter
Sleep 5s

Enter
Sleep 6s

Escape
Sleep 3s
