package ui

// Cue-editor catalogs are indexed by their corresponding show enum values.
// Keeping them together makes enum/dropdown alignment explicit and gives both
// cue forms and timecode marker forms one presentation source.
var (
	cueLinkModeLabels = []string{
		"Manual",
		"Start Advance",
		"Start Play",
		"Fade In Advance",
		"Fade In Play",
		"Fade Out Advance",
		"Fade Out Play",
		"End Advance",
		"End Play",
	}
	cueTargetKindLabels = []string{
		"None",
		"Next",
		"Previous",
		"Cue ID",
	}
	remoteProtocolLabels = []string{
		"Auto",
		"OSC",
		"ERC",
	}
	remoteActionLabels = []string{
		"None",
		"Go",
		"Go to",
		"Back",
		"Release",
		"Level",
		"Activate",
		"Flash",
		"Custom",
	}
	waitKindLabels = []string{
		"Duration",
		"Media Start",
		"Media End",
		"Fade In Complete",
		"Fade Out Complete",
		"Instance Stopped",
		"All Audio Stopped",
		"All Video Stopped",
		"All Media Stopped",
	}
	mediaTargetKindLabels = []string{
		"Cue ID",
		"Instance ID",
		"All Audio",
		"All Video",
		"All Media",
		"Output ID",
		"Current Track",
		"Cue Group",
	}
	mediaControlActionLabels = []string{
		"Fade To",
		"Fade Out",
		"Stop",
		"Pause",
		"Resume",
		"Seek",
		"Set Volume",
		"Mute",
		"Unmute",
	}
	fadeCurveLabels = []string{
		"Linear",
		"Equal Power",
	}
	outputControlActionLabels = []string{
		"Blackout",
		"Clear",
		"Test Pattern",
		"Identify",
		"Reopen",
		"Fullscreen",
		"Exit Fullscreen",
	}
	timecodeActionLabels = []string{
		"Current track",
		"Output control",
		"Remote",
	}

	videoFileExtensions = []string{".mp4", ".mov", ".mkv", ".webm", ".avi"}
	soundFileExtensions = []string{".wav", ".mp3", ".flac", ".ogg", ".aiff", ".aif", ".m4a", ".opus"}
	imageFileExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}
)
