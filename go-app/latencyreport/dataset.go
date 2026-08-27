package latencyreport

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

const (
	TransportLegacy = "legacy"
	TransportCDCV2  = "cdc-v2"
	TransportHIDV3  = "hid-v3"
)

var transportCSVHeader = append([]string{"transport"}, csvHeader...)

type Dataset struct {
	Transport string   `json:"transport"`
	Samples   []Sample `json:"-"`
}

// ParseDatasetCSV accepts both the transport-aware HIL schema and the original
// 9-column schema. Legacy data remains analyzable but is explicitly labelled
// "legacy" so it cannot be mistaken for a controlled CDC/HID A-B run.
func ParseDatasetCSV(reader io.Reader) (Dataset, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return Dataset{}, fmt.Errorf("read HIL header: %w", err)
	}

	transportAware := equalHeader(header, transportCSVHeader)
	legacy := equalHeader(header, csvHeader)
	if !transportAware && !legacy {
		return Dataset{}, fmt.Errorf("unexpected HIL header: %s", strings.Join(header, ","))
	}

	dataset := Dataset{Transport: TransportLegacy, Samples: make([]Sample, 0, 1024)}
	row := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		row++
		if err != nil {
			return Dataset{}, fmt.Errorf("read HIL row %d: %w", row, err)
		}

		payload := record
		if transportAware {
			if len(record) != len(transportCSVHeader) {
				return Dataset{}, fmt.Errorf("parse HIL row %d: unexpected column count %d", row, len(record))
			}
			transport, err := normalizeTransport(record[0])
			if err != nil {
				return Dataset{}, fmt.Errorf("parse HIL row %d: %w", row, err)
			}
			if dataset.Transport == TransportLegacy {
				dataset.Transport = transport
			} else if dataset.Transport != transport {
				return Dataset{}, fmt.Errorf("parse HIL row %d: mixed transports %q and %q", row, dataset.Transport, transport)
			}
			payload = record[1:]
		}

		sample, err := parseRecord(payload)
		if err != nil {
			return Dataset{}, fmt.Errorf("parse HIL row %d: %w", row, err)
		}
		dataset.Samples = append(dataset.Samples, sample)
	}
	return dataset, nil
}

func normalizeTransport(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TransportCDCV2:
		return TransportCDCV2, nil
	case TransportHIDV3:
		return TransportHIDV3, nil
	default:
		return "", fmt.Errorf("unsupported transport %q; expected %s or %s", value, TransportCDCV2, TransportHIDV3)
	}
}

func equalHeader(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
