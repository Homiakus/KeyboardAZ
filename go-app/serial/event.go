package serial

import "hapticpad-go-app/protocol"

// Event converts the CDC parser representation into the transport-neutral
// semantic model at the adapter boundary. This is intentionally lossless while
// connection.Runtime is still migrated from ButtonMessage to protocol.Event.
func (m ButtonMessage) Event() protocol.Event {
	return protocol.Event{
		Protocol:   m.Protocol,
		Type:       m.Type,
		Layer:      m.Layer,
		Buttons:    append([]int(nil), m.Buttons...),
		Mask:       m.Mask,
		Sequence:   m.Sequence,
		Firmware:   m.Firmware,
		Language:   m.Language,
		Modifiers:  m.Modifiers,
		Button:     m.Button,
		Action:     m.Action,
		ErrorCode:  m.ErrorCode,
		ErrorValue: m.ErrorValue,
		Armed:      m.Armed,
		ThumbMask:  m.ThumbMask,
		MainMask:   m.MainMask,
	}
}
