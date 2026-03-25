Cue Types: Light, Sound

# Sound
Sound cue types: Play once, Vamp
All have:
    Options:
        - Clip (upload as well)
        - Play style (play alongside other clips, wait till all other clips finish -- show that it is automatically gonna start, fade out all clips then start, xfade with end of other clips -- show it will start, )
        - Clip start (default 0s)
        - Clip end (default {len}s)
        - Fade in (default 0s)
        - Fade out (default 0s)
        - Auto after (default do nothing, but can trigger another cue)
        - Default manual f/o duration (default 2s)
        - Volume (+/- db)
        - Allow multiple instances of clip
    Cue actions (shown when clip is during playback):
        - Fade out (w/ manual f/o duration option, defautls to selected)
        - Stop

Vamp:
    Options:
        - Devamp action (jump to end of loop, fade to end of loop w/ fade duration option, play out, fade out w/ duration)
        - Loop xfade (default 0s)
        - Loop start (default 0s)
        - Loop end (default {len}s)
    Cue actions:
        - Devamp

# Lights
