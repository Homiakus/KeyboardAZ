package hidv3

import (
	"errors"
	"math"
	"testing"
	"time"

	"hapticpad-go-app/transport"
)

func encodedInputReport(t *testing.T, report transport.ReportV3) []byte {
	t.Helper()
	payload, err := transport.EncodeV3(report)
	if err != nil {
		t.Fatalf("EncodeV3: %v", err)
	}
	input := make([]byte, InputReportSize)
	input[0] = 7
	copy(input[1:], payload[:])
	return input
}

func TestDecodeInputReportPreservesClockDomains(t *testing.T) {
	report := transport.ReportV3{
		Type:             transport.EventStroke,
		Language:         transport.LanguageRussian,
		ButtonOrAction:   17,
		Modifiers:        0x09,
		Sequence:         0x10203040,
		EventTimestampUS: math.MaxUint32,
	}
	hostReceivedAt := time.Unix(1_800_000_000, 123_456_789)

	event, observation, err := DecodeInputReport(encodedInputReport(t, report), hostReceivedAt)
	if err != nil {
		t.Fatalf("DecodeInputReport: %v", err)
	}
	if observation.Report != report {
		t.Fatalf("report changed: got %+v want %+v", observation.Report, report)
	}
	if !observation.HostReceivedAt.Equal(hostReceivedAt) {
		t.Fatalf("host receive time changed: got %v want %v", observation.HostReceivedAt, hostReceivedAt)
	}
	if event.Sequence != report.Sequence || event.Type != "stroke" || event.Button != 17 {
		t.Fatalf("unexpected semantic event: %+v", event)
	}
}

func TestDecodeInputReportRejectsZeroReportID(t *testing.T) {
	input := encodedInputReport(t, transport.ReportV3{
		Type:     transport.EventLanguage,
		Language: transport.LanguageEnglish,
		Sequence: 1,
	})
	input[0] = 0
	if _, _, err := DecodeInputReport(input, time.Now()); err == nil {
		t.Fatal("expected zero report ID to be rejected")
	}
}

func TestDecodeInputReportRejectsWrongSize(t *testing.T) {
	if _, _, err := DecodeInputReport(make([]byte, InputReportSize-1), time.Now()); err == nil {
		t.Fatal("expected short HID report to be rejected")
	}
}

func TestObserverFuncReceivesObservationAndReturnsErrors(t *testing.T) {
	want := Observation{
		Report: transport.ReportV3{
			Type:             transport.EventTap,
			Language:         transport.LanguageEnglish,
			ButtonOrAction:   uint8(transport.TapEnter),
			Sequence:         42,
			EventTimestampUS: 77,
		},
		HostReceivedAt: time.Unix(123, 456),
	}
	wantErr := errors.New("disk full")
	var got Observation
	observer := ObserverFunc(func(observation Observation) error {
		got = observation
		return wantErr
	})
	if err := observer.ObserveHIDV3(want); !errors.Is(err, wantErr) {
		t.Fatalf("observer error=%v want %v", err, wantErr)
	}
	if got != want {
		t.Fatalf("observer changed observation: got %+v want %+v", got, want)
	}

	var nilObserver ObserverFunc
	if err := nilObserver.ObserveHIDV3(want); err != nil {
		t.Fatalf("nil observer returned error: %v", err)
	}
}

func BenchmarkDecodeInputReport(b *testing.B) {
	report := transport.ReportV3{
		Type:             transport.EventStroke,
		Language:         transport.LanguageEnglish,
		ButtonOrAction:   3,
		Sequence:         1,
		EventTimestampUS: 100,
	}
	payload, err := transport.EncodeV3(report)
	if err != nil {
		b.Fatal(err)
	}
	input := make([]byte, InputReportSize)
	input[0] = 7
	copy(input[1:], payload[:])
	receivedAt := time.Unix(1_800_000_000, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := DecodeInputReport(input, receivedAt); err != nil {
			b.Fatal(err)
		}
	}
}
