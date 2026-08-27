package protocol

// Event is the transport-neutral semantic event consumed by application code.
// CDC v2, HID v3 and test fixtures should all translate into this type before
// crossing the application boundary.
type Event struct {
	Protocol int
	Type     string

	// Legacy protocol-v1 compatibility fields. They can be removed after the v1
	// migration window closes without changing the rest of the application API.
	Layer   int
	Buttons []int
	Mask    uint32

	// Semantic protocol fields shared by v2/v3 adapters.
	Sequence   uint32
	Firmware   string
	Language   string
	Modifiers  uint8
	Button     int
	Action     string
	ErrorCode  string
	ErrorValue uint32
	Armed      bool
	ThumbMask  uint8
	MainMask   uint32
}

func (e Event) IsSemantic() bool { return e.Protocol >= 2 }

func (e Event) IsPhysicalInput() bool {
	if e.Protocol >= 2 {
		return e.Type == "stroke" || e.Type == "tap"
	}
	return e.Type == "press" || e.Type == "combo"
}

func (e Event) IsHandshakeEvidence() bool {
	return e.Protocol == 2 && (e.Type == "ready" || e.Type == "status")
}

// Clone prevents UI/history/capture consumers from sharing mutable button
// slices with a transport adapter.
func (e Event) Clone() Event {
	e.Buttons = append([]int(nil), e.Buttons...)
	return e
}

// Event is a temporary source-compatibility bridge for code that previously
// converted serial.ButtonMessage into protocol.Event. serial.ButtonMessage is
// now an alias of Event, so this method costs only the intentional slice clone
// and can be removed once call sites consume Event directly.
func (e Event) Event() Event { return e.Clone() }
