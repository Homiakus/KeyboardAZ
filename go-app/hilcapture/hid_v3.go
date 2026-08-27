package hilcapture

import (
	"fmt"
	"io"
	"sync"

	"hapticpad-go-app/handler"
	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/latencyreport"
	"hapticpad-go-app/transport"
)

// Stats describes only capture mechanics; it never contains typed content.
type Stats struct {
	Captured          int
	SendInputObserved int
	SendInputFailures int
	Flushed           int
}

// HIDV3CSVObserver correlates Raw HID T1/T2 metadata and the first actual
// Windows SendInput T3 boundary by transport sequence. Samples stay in memory
// until Flush so no post-hoc file rewrite or timing heuristic is required.
// A 10k-stroke HIL run is intentionally small enough for this bounded workflow.
type HIDV3CSVObserver struct {
	mu      sync.Mutex
	writer  *latencyreport.DatasetWriter
	samples []latencyreport.Sample
	index   map[uint32]int
	flushed int
	err     error
	stats   Stats
}

func NewHIDV3CSVObserver(output io.Writer) (*HIDV3CSVObserver, error) {
	writer, err := latencyreport.NewDatasetWriter(output, latencyreport.TransportHIDV3)
	if err != nil {
		return nil, err
	}
	return &HIDV3CSVObserver{
		writer: writer,
		index:  make(map[uint32]int, 1024),
	}, nil
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
	if o.err != nil {
		return o.err
	}
	if _, exists := o.index[sample.Sequence]; exists {
		err := fmt.Errorf("duplicate HID v3 sequence %d in HIL capture", sample.Sequence)
		o.err = err
		return err
	}
	o.index[sample.Sequence] = len(o.samples)
	o.samples = append(o.samples, sample)
	o.stats.Captured++
	return nil
}

// ObserveSendInput implements handler.SendInputObserver. Only HID-v3 traces are
// consumed; other transports may share the same handler without contaminating
// this dataset. Observer callbacks never contain resolved text or key names.
func (o *HIDV3CSVObserver) ObserveSendInput(observation handler.SendInputObservation) {
	if o == nil || observation.Trace.Transport != latencyreport.TransportHIDV3 {
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err != nil {
		return
	}
	if !observation.Trace.Valid() || observation.CalledAt.IsZero() {
		o.err = fmt.Errorf("invalid SendInput observation for HIL capture")
		return
	}
	index, ok := o.index[observation.Trace.Sequence]
	if !ok {
		o.err = fmt.Errorf("SendInput sequence %d has no HID observation", observation.Trace.Sequence)
		return
	}
	if index < o.flushed {
		o.err = fmt.Errorf("SendInput sequence %d arrived after its HIL row was flushed", observation.Trace.Sequence)
		return
	}
	if o.samples[index].T3SendInputNS != 0 {
		o.err = fmt.Errorf("duplicate SendInput observation for sequence %d", observation.Trace.Sequence)
		return
	}
	o.samples[index].T3SendInputNS = observation.CalledAt.UnixNano()
	o.stats.SendInputObserved++
	if !observation.Success {
		o.stats.SendInputFailures++
	}
}

func (o *HIDV3CSVObserver) Err() error {
	if o == nil {
		return fmt.Errorf("HID v3 HIL observer is nil")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

func (o *HIDV3CSVObserver) Stats() Stats {
	if o == nil {
		return Stats{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	stats := o.stats
	stats.Flushed = o.flushed
	return stats
}

func (o *HIDV3CSVObserver) Flush() error {
	if o == nil || o.writer == nil {
		return fmt.Errorf("HID v3 HIL observer is not initialized")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.err != nil {
		return o.err
	}
	for o.flushed < len(o.samples) {
		if err := o.writer.WriteSample(o.samples[o.flushed]); err != nil {
			o.err = err
			return err
		}
		o.flushed++
	}
	if err := o.writer.Flush(); err != nil {
		o.err = err
		return err
	}
	o.stats.Flushed = o.flushed
	return nil
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
var _ handler.SendInputObserver = (*HIDV3CSVObserver)(nil)
