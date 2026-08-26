package transport

import (
	"encoding/binary"
	"fmt"
)

const (
	ProtocolV3Version = 3
	ProtocolV3Size    = 16
)

type EventType uint8

const (
	EventStroke   EventType = 1
	EventTap      EventType = 2
	EventLanguage EventType = 3
)

type Language uint8

const (
	LanguageEnglish Language = 0
	LanguageRussian Language = 1
)

type TapAction uint8

const (
	TapSpace     TapAction = 1
	TapEnter     TapAction = 2
	TapBackspace TapAction = 3
)

// ReportV3 is the fixed-size realtime report transported over the future Raw
// HID interrupt endpoint. Sequence zero is reserved and never emitted.
type ReportV3 struct {
	Type             EventType
	Flags            uint8
	Language         Language
	ButtonOrAction   uint8
	Modifiers        uint8
	Sequence         uint32
	EventTimestampUS uint32
}

// EncodeV3 produces one allocation-free 16-byte little-endian report.
func EncodeV3(report ReportV3) ([ProtocolV3Size]byte, error) {
	var encoded [ProtocolV3Size]byte
	if err := ValidateV3(report); err != nil {
		return encoded, err
	}

	encoded[0] = ProtocolV3Version
	encoded[1] = byte(report.Type)
	encoded[2] = report.Flags
	encoded[3] = byte(report.Language)
	encoded[4] = report.ButtonOrAction
	encoded[5] = report.Modifiers
	// bytes 6..7 are reserved for forward-compatible flags/schema growth.
	binary.LittleEndian.PutUint32(encoded[8:12], report.Sequence)
	binary.LittleEndian.PutUint32(encoded[12:16], report.EventTimestampUS)
	return encoded, nil
}

func DecodeV3(data []byte) (ReportV3, error) {
	if len(data) != ProtocolV3Size {
		return ReportV3{}, fmt.Errorf("protocol v3 report size %d, want %d", len(data), ProtocolV3Size)
	}
	if data[0] != ProtocolV3Version {
		return ReportV3{}, fmt.Errorf("protocol version %d, want %d", data[0], ProtocolV3Version)
	}
	if data[6] != 0 || data[7] != 0 {
		return ReportV3{}, fmt.Errorf("protocol v3 reserved bytes must be zero")
	}

	report := ReportV3{
		Type:             EventType(data[1]),
		Flags:            data[2],
		Language:         Language(data[3]),
		ButtonOrAction:   data[4],
		Modifiers:        data[5],
		Sequence:         binary.LittleEndian.Uint32(data[8:12]),
		EventTimestampUS: binary.LittleEndian.Uint32(data[12:16]),
	}
	if err := ValidateV3(report); err != nil {
		return ReportV3{}, err
	}
	return report, nil
}

func ValidateV3(report ReportV3) error {
	if report.Sequence == 0 {
		return fmt.Errorf("protocol v3 sequence zero is reserved")
	}
	if report.Language != LanguageEnglish && report.Language != LanguageRussian {
		return fmt.Errorf("protocol v3 invalid language %d", report.Language)
	}
	if report.Modifiers&^uint8(0x0F) != 0 {
		return fmt.Errorf("protocol v3 invalid modifier bits 0x%02X", report.Modifiers)
	}

	switch report.Type {
	case EventStroke:
		if report.ButtonOrAction > 21 {
			return fmt.Errorf("protocol v3 invalid button %d", report.ButtonOrAction)
		}
	case EventTap:
		action := TapAction(report.ButtonOrAction)
		if action != TapSpace && action != TapEnter && action != TapBackspace {
			return fmt.Errorf("protocol v3 invalid tap action %d", report.ButtonOrAction)
		}
	case EventLanguage:
		if report.ButtonOrAction != 0 || report.Modifiers != 0 {
			return fmt.Errorf("protocol v3 language event carries unexpected payload")
		}
	default:
		return fmt.Errorf("protocol v3 unknown event type %d", report.Type)
	}
	return nil
}
