package latencyreport

import (
	"bytes"
	"testing"
)

func TestDatasetWriterRoundTripsTransportAwareSamples(t *testing.T) {
	var buffer bytes.Buffer
	writer, err := NewDatasetWriter(&buffer, TransportHIDV3)
	if err != nil {
		t.Fatalf("NewDatasetWriter: %v", err)
	}
	want := Sample{
		Sequence:     17,
		T1FirmwareUS: ^uint32(0),
		T2HostRxNS:   1_800_000_000_123_456_789,
		EventType:    "stroke",
		Button:       4,
		Modifiers:    0x09,
	}
	if err := writer.WriteSample(want); err != nil {
		t.Fatalf("WriteSample: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	dataset, err := ParseDatasetCSV(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatalf("ParseDatasetCSV: %v\n%s", err, buffer.String())
	}
	if dataset.Transport != TransportHIDV3 {
		t.Fatalf("transport=%q", dataset.Transport)
	}
	if len(dataset.Samples) != 1 || dataset.Samples[0] != want {
		t.Fatalf("round trip changed sample: got %+v want %+v", dataset.Samples, want)
	}
}

func TestDatasetWriterRejectsInvalidSamples(t *testing.T) {
	var buffer bytes.Buffer
	writer, err := NewDatasetWriter(&buffer, TransportCDCV2)
	if err != nil {
		t.Fatalf("NewDatasetWriter: %v", err)
	}
	cases := []Sample{
		{Sequence: 0, EventType: "stroke", Button: 1},
		{Sequence: 1, Button: 1},
		{Sequence: 1, EventType: "stroke", Button: 22},
		{Sequence: 1, EventType: "stroke", Button: 1, Modifiers: 0x10},
		{Sequence: 1, EventType: "stroke", Button: 1, T2HostRxNS: -1},
	}
	for _, sample := range cases {
		if err := writer.WriteSample(sample); err == nil {
			t.Fatalf("expected rejection for %+v", sample)
		}
	}
}

func TestDatasetWriterRejectsUnsupportedTransport(t *testing.T) {
	var buffer bytes.Buffer
	if _, err := NewDatasetWriter(&buffer, "auto"); err == nil {
		t.Fatal("expected unsupported transport rejection")
	}
}
