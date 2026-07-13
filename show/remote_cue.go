package show

type RemotePlay struct {
	Protocol RemoteProtocol `json:"protocol"`
	Action   RemoteAction   `json:"action"`

	Playback  string `json:"playback,omitempty"`
	CueNumber string `json:"cueNumber,omitempty"`

	Level  string        `json:"level,omitempty"`
	Custom string        `json:"custom,omitempty"`
	Values []RemoteValue `json:"values,omitempty"`
}

type RemoteProtocol int

const (
	RemoteProtocolAuto RemoteProtocol = iota
	RemoteProtocolOSC
	RemoteProtocolERC
)

type RemoteAction int

const (
	RemoteActionNone RemoteAction = iota
	RemoteActionGo
	RemoteActionGoto
	RemoteActionBack
	RemoteActionRelease
	RemoteActionLevel
	RemoteActionActivate
	RemoteActionFlash
	RemoteActionCustom
)

type RemoteValue struct {
	Type  RemoteValueType `json:"type"`
	Value string          `json:"value"`
}

type RemoteValueType int

const (
	RemoteValueString RemoteValueType = iota
	RemoteValueInt
	RemoteValueFloat
	RemoteValueBool
)
