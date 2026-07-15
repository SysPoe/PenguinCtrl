package media

import "math"

const (
	muteFloorDB = -80.0
	maxGainDB   = 12.0
)

var maxGainLinear = math.Pow(10, maxGainDB/20)

func dbVolume(db float64, muted bool) float64 {
	if muted || db <= muteFloorDB {
		return 0
	}
	return min(maxGainLinear, math.Pow(10, db/20))
}
