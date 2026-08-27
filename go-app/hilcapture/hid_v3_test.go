package hilcapture

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"hapticpad-go-app/handler"
	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/latencyreport"
	"hapticpad-go-app/transport"
)

func TestHIDV3CSVObserverWritesCanonicalDataset(t *testing.T) {
	var buffer bytes.Buffer
	observer, err := NewHIDV3CSVObserver(&buffer)
	if err != nil {
		t.Fatalf("NewHIDV3CSVObserver: %v", err)
	}
	receivedAt := time.Unix(1_800_000_000, 123_456_789)
	observation := hidv3.Observation{
		Report: transport.ReportV3{
			Type:             transport.EventStroke,
			Language:         transport.LanguageRussian,
			ButtonOrAction:   17,
			Modifiers:        0x09,
			Sequence:         77,
			EventTimestampUS: ^uint32(0),
		},
		HostReceivedAt: receivedAt,
	}
	if err := observer.ObserveHIDV3(observation); err != nil {
		t.Fatalf("ObserveHIDV3: %v", err)
	}
	if err := observer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	dataset, err := latencyreport.ParseDatasetCSV(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("ParseDatasetCSV: %v\n%s", err, buffer.String())
	}
	if dataset.Transport != latencyreport.TransportHIDV3 || len(dataset.Samples) != 1 {
		t.Fatalf("unexpected dataset: %+v", dataset)
	}
	sample := dataset.Samples[0]
	if sample.Sequence != 77 || sample.T1FirmwareUS != ^uint32(0) || sample.T2HostRxNS != receivedAt.UnixNano() {
		t.Fatalf("timing metadata changed: %+v", sample)
	}
	if sample.T0FixtureNS != 0 || sample.T3SendInputNS != 0 || sample.T4FixtureNS != 0 {
		t.Fatalf("unmeasured stages must remain zero: %+v", sample)
	}
	if sample.EventType != "stroke" || sample.Button != 17 || sample.Modifiers != 0x09 {
		t.Fatalf("semantic mapping changed: %+v", sample)
	}
	stats := observer.Stats()
	if stats.Captured != 1 || stats.HostTimingExpected != 1 || stats.Flushed != 1 {
		t.Fatalf("unexpected capture stats: %+v", stats)
	}
}

func TestHIDV3CSVObserverCorrelatesSendInputBySequence(t *testing.T) {
	var buffer bytes.Buffer
	observer, err := NewHIDV3CSVObserver(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	hostReceivedAt := time.Unix(1_800_000_000, 100)
	sendInputAt := hostReceivedAt.Add(750 * time.Microsecond)
	if err := observer.ObserveHIDV3(hidv3.Observation{
		Report: transport.ReportV3{
			Type:             transport.EventStroke,
			Language:         transport.LanguageEnglish,
			ButtonOrAction:   3,
			Sequence:         9,
			EventTimestampUS: 12345,
		},
		HostReceivedAt: hostReceivedAt,
	}); err != nil {
		t.Fatal(err)
	}
	observer.ObserveSendInput(handler.SendInputObservation{
		Trace:    handler.InputTrace{Transport: latencyreport.TransportHIDV3, Sequence: 9},
		CalledAt: sendInputAt,
		Success:  true,
	})
	if err := observer.Err(); err != nil {
		t.Fatalf("unexpected correlation error: %v", err)
	}
	if err := observer.Flush(); err != nil {
		t.Fatal(err)
	}

	dataset, err := latencyreport.ParseDatasetCSV(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	sample := dataset.Samples[0]
	if sample.T2HostRxNS != hostReceivedAt.UnixNano() || sample.T3SendInputNS != sendInputAt.UnixNano() {
		t.Fatalf("unexpected T2/T3 correlation: %+v", sample)
	}
	stats := observer.Stats()
	if stats.Captured != 1 || stats.HostTimingExpected != 1 || stats.SendInputObserved != 1 || stats.SendInputFailures != 0 || stats.Flushed != 1 {
		t.Fatalf("unexpected capture stats: %+v", stats)
	}
}

func TestHIDV3CSVObserverDetectsUnmatchedSendInput(t *testing.T) {
	var buffer bytes.Buffer
	observer, err := NewHIDV3CSVObserver(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	observer.ObserveSendInput(handler.SendInputObservation{
		Trace:    handler.InputTrace{Transport: latencyreport.TransportHIDV3, Sequence: 44},
		CalledAt: time.Now(),
		Success:  true,
	})
	if observer.Err() == nil {
		t.Fatal("expected unmatched SendInput correlation error")
	}
	if err := observer.Flush(); err == nil {
		t.Fatal("correlation error must make capture flush fail closed")
	}
}

func TestHIDV3CSVObserverMapsNonStrokeWithoutFakeButton(t *testing.T) {
	for _, eventType := range []transport.EventType{transport.EventTap, transport.EventLanguage} {
		var buffer bytes.Buffer
		observer, err := NewHIDV3CSVObserver(&buffer)
		if err != nil {
			t.Fatal(err)
		}
		report := transport.ReportV3{
			Type:             eventType,
			Language:         transport.LanguageEnglish,
			Sequence:         1,
			EventTimestampUS: 10,
		}
		if eventType == transport.EventTap {
			report.ButtonOrAction = uint8(transport.TapEnter)
		}
		if err := observer.ObserveHIDV3(hidv3.Observation{Report: report, HostReceivedAt: time.Unix(100, 0)}); err != nil {
			t.Fatal(err)
		}
		if err := observer.Flush(); err != nil {
			t.Fatal(err)
		}
		dataset, err := latencyreport.ParseDatasetCSV(bytes.NewReader(buffer.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if dataset.Samples[0].Button != -1 {
			t.Fatalf("event %d got fake button %d", eventType, dataset.Samples[0].Button)
		}
		stats := observer.Stats()
		wantHost := 0
		if eventType == transport.EventTap {
			wantHost = 1
		}
		if stats.HostTimingExpected != wantHost {
			t.Fatalf("event %d host timing expected=%d want=%d", eventType, stats.HostTimingExpected, wantHost)
		}
	}
}

func TestHIDV3CSVObserverEnforcesCaptureLimitBeforeExtraSample(t *testing.T) {
	var buffer bytes.Buffer
	observer, err := NewHIDV3CSVObserverWithLimit(&buffer, 2)
	if err != nil {
		t.Fatal(err)
	}
	makeObservation := func(sequence uint32) hidv3.Observation {
		return hidv3.Observation{
			Report: transport.ReportV3{
				Type:             transport.EventLanguage,
				Language:         transport.LanguageEnglish,
				Sequence:         sequence,
				EventTimestampUS: sequence * 10,
			},
			HostReceivedAt: time.Unix(100, int64(sequence)),
		}
	}
	if err := observer.ObserveHIDV3(makeObservation(1)); err != nil {
		t.Fatal(err)
	}
	if err := observer.ObserveHIDV3(makeObservation(2)); err != nil {
		t.Fatal(err)
	}
	if err := observer.ObserveHIDV3(makeObservation(3)); !errors.Is(err, ErrCaptureLimitReached) {
		t.Fatalf("expected capture limit sentinel, got %v", err)
	}
	if stats := observer.Stats(); stats.Captured != 2 {
		t.Fatalf("capture exceeded limit: %+v", stats)
	}
	if err := observer.Flush(); err != nil {
		t.Fatal(err)
	}
	dataset, err := latencyreport.ParseDatasetCSV(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Samples) != 2 || dataset.Samples[1].Sequence != 2 {
		t.Fatalf("unexpected bounded dataset: %+v", dataset.Samples)
	}
}

func TestHIDV3CSVObserverRejectsMissingHostTimestamp(t *testing.T) {
	var buffer bytes.Buffer
	observer, err := NewHIDV3CSVObserver(&buffer)
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.ObserveHIDV3(hidv3.Observation{Report: transport.ReportV3{Type: transport.EventLanguage, Language: transport.LanguageEnglish, Sequence: 1}}); err == nil {
		t.Fatal("expected missing host timestamp rejection")
	}
}
