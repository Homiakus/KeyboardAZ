package latencyreport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

// DatasetWriter emits the transport-aware HIL schema consumed by
// ParseDatasetCSV. Missing measurement stages are represented by zero exactly as
// the parser/summarizer already define; callers must never synthesize timings.
type DatasetWriter struct {
	csv       *csv.Writer
	transport string
}

func NewDatasetWriter(writer io.Writer, transport string) (*DatasetWriter, error) {
	if writer == nil {
		return nil, fmt.Errorf("HIL writer is nil")
	}
	normalized, err := normalizeTransport(transport)
	if err != nil {
		return nil, err
	}
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(transportCSVHeader); err != nil {
		return nil, fmt.Errorf("write HIL header: %w", err)
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, fmt.Errorf("flush HIL header: %w", err)
	}
	return &DatasetWriter{csv: csvWriter, transport: normalized}, nil
}

func (w *DatasetWriter) WriteSample(sample Sample) error {
	if w == nil || w.csv == nil {
		return fmt.Errorf("HIL dataset writer is not initialized")
	}
	if sample.Sequence == 0 {
		return fmt.Errorf("sequence zero is reserved")
	}
	if sample.EventType == "" {
		return fmt.Errorf("event type is required")
	}
	if sample.Button < -1 || sample.Button > 21 {
		return fmt.Errorf("button %d outside canonical range -1..21", sample.Button)
	}
	if sample.Modifiers&^uint8(0x0F) != 0 {
		return fmt.Errorf("unsupported modifier bits 0x%02X", sample.Modifiers)
	}
	if sample.T0FixtureNS < 0 || sample.T2HostRxNS < 0 || sample.T3SendInputNS < 0 || sample.T4FixtureNS < 0 {
		return fmt.Errorf("timestamps must be non-negative")
	}

	record := []string{
		w.transport,
		strconv.FormatUint(uint64(sample.Sequence), 10),
		strconv.FormatInt(sample.T0FixtureNS, 10),
		strconv.FormatUint(uint64(sample.T1FirmwareUS), 10),
		strconv.FormatInt(sample.T2HostRxNS, 10),
		strconv.FormatInt(sample.T3SendInputNS, 10),
		strconv.FormatInt(sample.T4FixtureNS, 10),
		sample.EventType,
		strconv.Itoa(sample.Button),
		strconv.FormatUint(uint64(sample.Modifiers), 10),
	}
	if err := w.csv.Write(record); err != nil {
		return fmt.Errorf("write HIL sample: %w", err)
	}
	return nil
}

func (w *DatasetWriter) Flush() error {
	if w == nil || w.csv == nil {
		return fmt.Errorf("HIL dataset writer is not initialized")
	}
	w.csv.Flush()
	if err := w.csv.Error(); err != nil {
		return fmt.Errorf("flush HIL dataset: %w", err)
	}
	return nil
}
