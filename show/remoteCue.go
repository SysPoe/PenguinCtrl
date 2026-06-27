package show

type RemotePlay struct {
	Protocol RemoteProtocol
	Action   RemoteAction

	Playback  string
	CueNumber string

	Level string

	Address string
	Args    []RemoteValue

	Host string
	Port int
}

type RemoteProtocol int

const (
	RemoteProtocolOSC RemoteProtocol = iota
	RemoteProtocolERC
	RemoteProtocolAuto
)

type RemoteAction int

const (
	RemoteActionNone RemoteAction = iota
	RemoteActionGo
	RemoteActionGoto
	RemoteActionBack
	RemoteActionRelease
	RemoteActionLevel
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