package show

type RemotePlay struct {
	Protocol RemoteProtocol
	Action   RemoteAction

	Playback  string
	CueNumber string

	Level  string
	Custom string
	Values []RemoteValue
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
	Type  RemoteValueType
	Value string
}

type RemoteValueType int

const (
	RemoteValueString RemoteValueType = iota
	RemoteValueInt
	RemoteValueFloat
	RemoteValueBool
)
