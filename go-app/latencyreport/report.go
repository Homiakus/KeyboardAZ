package latencyreport

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"time"
)

var csvHeader = []string{
	"sequence",
	"t0_fixture_ns",
	"t1_firmware_us",
	"t2_host_rx_ns",
	"t3_sendinput_ns",
	"t4_fixture_ns",
	"event_type",
	"button",
	"modifiers",
}

type Sample struct {
	Sequence     uint32
	T0FixtureNS  int64
	T1FirmwareUS uint32
	T2HostRxNS   int64
	T3SendInputNS int64
	T4FixtureNS  int64
	EventType    string
	Button       int
	Modifiers    uint8
}

type Distribution struct {
	Count int           `json:"count"`
	P50   time.Duration `json:"p50"`
	P95   time.Duration `json:"p95"`
	P99   time.Duration `json:"p99"`
	Max   time.Duration `json:"max"`
}

type Summary struct {
	Samples            int          `json:"samples"`
	SequenceGaps       uint64       `json:"sequence_gaps"`
	SequenceDuplicates uint64       `json:"sequence_duplicates"`
	SequenceOutOfOrder uint64       `json:"sequence_out_of_order"`
	HostDispatch       Distribution `json:"host_rx_to_sendinput"`
	FixtureE2E         Distribution `json:"fixture_e2e"`
}

func ParseCSV(reader io.Reader) ([]Sample, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read HIL header: %w", err)
	}
	if len(header) != len(csvHeader) {
		return nil, fmt.Errorf("unexpected HIL column count %d", len(header))
	}
	for i := range csvHeader {
		if header[i] != csvHeader[i] {
			return nil, fmt.Errorf("unexpected HIL column %d: got %q want %q", i, header[i], csvHeader[i])
		}
	}

	result := make([]Sample, 0, 1024)
	row := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		row++
		if err != nil {
			return nil, fmt.Errorf("read HIL row %d: %w", row, err)
		}
		sample, err := parseRecord(record)
		if err != nil {
			return nil, fmt.Errorf("parse HIL row %d: %w", row, err)
		}
		result = append(result, sample)
	}
	return result, nil
}

func parseRecord(record []string) (Sample, error) {
	if len(record) != len(csvHeader) {
		return Sample{}, fmt.Errorf("unexpected column count %d", len(record))
	}

	sequence, err := parseUint(record[0], 32)
	if err != nil {
		return Sample{}, fmt.Errorf("sequence: %w", err)
	}
	t0, err := strconv.ParseInt(record[1], 10, 64)
	if err != nil {
		return Sample{}, fmt.Errorf("t0_fixture_ns: %w", err)
	}
	t1, err := parseUint(record[2], 32)
	if err != nil {
		return Sample{}, fmt.Errorf("t1_firmware_us: %w", err)
	}
	t2, err := strconv.ParseInt(record[3], 10, 64)
	if err != nil {
		return Sample{}, fmt.Errorf("t2_host_rx_ns: %w", err)
	}
	t3, err := strconv.ParseInt(record[4], 10, 64)
	if err != nil {
		return Sample{}, fmt.Errorf("t3_sendinput_ns: %w", err)
	}
	t4, err := strconv.ParseInt(record[5], 10, 64)
	if err != nil {
		return Sample{}, fmt.Errorf("t4_fixture_ns: %w", err)
	}
	button, err := strconv.Atoi(record[7])
	if err != nil {
		return Sample{}, fmt.Errorf("button: %w", err)
	}
	modifiers, err := parseUint(record[8], 8)
	if err != nil {
		return Sample{}, fmt.Errorf("modifiers: %w", err)
	}

	return Sample{
		Sequence:      uint32(sequence),
		T0FixtureNS:   t0,
		T1FirmwareUS:  uint32(t1),
		T2HostRxNS:    t2,
		T3SendInputNS: t3,
		T4FixtureNS:   t4,
		EventType:     record[6],
		Button:        button,
		Modifiers:     uint8(modifiers),
	}, nil
}

func parseUint(value string, bits int) (uint64, error) {
	return strconv.ParseUint(value, 10, bits)
}

func Summarize(samples []Sample) Summary {
	summary := Summary{Samples: len(samples)}
	host := make([]time.Duration, 0, len(samples))
	e2e := make([]time.Duration, 0, len(samples))

	var last uint32
	initialized := false
	for _, sample := range samples {
		if sample.T2HostRxNS > 0 && sample.T3SendInputNS >= sample.T2HostRxNS {
			host = append(host, time.Duration(sample.T3SendInputNS-sample.T2HostRxNS))
		}
		if sample.T0FixtureNS > 0 && sample.T4FixtureNS >= sample.T0FixtureNS {
			e2e = append(e2e, time.Duration(sample.T4FixtureNS-sample.T0FixtureNS))
		}

		if sample.Sequence == 0 {
			continue
		}
		if !initialized {
			last = sample.Sequence
			initialized = true
			continue
		}
		if sample.Sequence == last {
			summary.SequenceDuplicates++
			continue
		}
		if sample.Sequence > last {
			if delta := uint64(sample.Sequence - last); delta > 1 {
				summary.SequenceGaps += delta - 1
			}
			last = sample.Sequence
			continue
		}

		// Treat only a high->low boundary as uint32 wrap. A normal backwards
		// jump is reported separately rather than exploding gap counts.
		if last >= 0xF0000000 && sample.Sequence <= 0x0FFFFFFF {
			delta := uint64(^uint32(0)-last) + uint64(sample.Sequence)
			if delta > 1 {
				summary.SequenceGaps += delta - 1
			}
			last = sample.Sequence
			continue
		}
		summary.SequenceOutOfOrder++
	}

	summary.HostDispatch = summarizeDistribution(host)
	summary.FixtureE2E = summarizeDistribution(e2e)
	return summary
}

func summarizeDistribution(values []time.Duration) Distribution {
	if len(values) == 0 {
		return Distribution{}
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return Distribution{
		Count: len(sorted),
		P50:   percentile(sorted, 0.50),
		P95:   percentile(sorted, 0.95),
		P99:   percentile(sorted, 0.99),
		Max:   sorted[len(sorted)-1],
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(p*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
