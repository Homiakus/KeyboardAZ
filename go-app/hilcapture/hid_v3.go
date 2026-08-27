package hilcapture

import (
	"fmt"
	"io"
	"sync"

	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/latencyreport"
	"hapticpad-go-app/transport"
)

// HIDV3CSVObserver converts lossless Raw HID observations into the canonical
// transport-aware HIL CSV schema. Only stages actually observed here are
// populated: T1 firmware micros() and T2 host receive time. Fixture and
// SendInput stages remain zero until a correlated recorder supplies them.
type HIDV3CSVObserver struct {
	mu     sync.Mutex
	writer *latencyreport.DatasetWriter
}

func NewHIDV3CSVObserver(output io.Writer) (*HIDV3CSVObserver, error) {
	writer, err := latencyreport.NewDatasetWriter(output, latencyreport.TransportHIDV3)
	if err != nil {
		return nil, err
	}
	return &HIDV3CSVObserver{writer: writer}, nil
}

func (o *HIDV3CSVObserver) ObserveHIDV3(observation hidv3.Observation) error {
	if o == nil || o.writer == nil {
		return fmt.Errorf("HID v3 HIL observer is not initialized")
	}
	if observation.HostReceivedAt.IsZero() {
		return fmt.Errorf("HID v3 observation has no host receive timestamp")
	}

	eventType, button, err := semanticFields(observation.Report)
	if err != nil {
		return err
	}
	sample := latencyreport.Sample{
		Sequence:     observation.Report.Sequence,
		T1FirmwareUS: observation.Report.EventTimestampUS,
		T2HostRxNS:   observation.HostReceivedAt.UnixNano(),
		EventType:    eventType,
		Button:       button,
		Modifiers:    observation.Report.Modifiers,
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	return o.writer.WriteSample(sample)
}

func (o *HIDV3CSVObserver) Flush() error {
	if o == nil || o.writer == nil {
		return fmt.Errorf("HID v3 HIL observer is not initialized")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.writer.Flush()
}

func semanticFields(report transport.ReportV3) (eventType string, button int, err error) {
	switch report.Type {
	case transport.EventStroke:
		return "stroke", int(report.ButtonOrAction), nil
	case transport.EventTap:
		return "tap", -1, nil
	case transport.EventLanguage:
		return "language", -1, nil
	default:
		return "", -1, fmt.Errorf("unsupported HID v3 event type %d", report.Type)
	}
}

var _ hidv3.Observer = (*HIDV3CSVObserver)(nil)
