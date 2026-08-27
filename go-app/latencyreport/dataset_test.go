package latencyreport

import (
	"strings"
	"testing"
)

const hilPayloadHeader = "sequence,t0_fixture_ns,t1_firmware_us,t2_host_rx_ns,t3_sendinput_ns,t4_fixture_ns,event_type,button,modifiers"

func TestParseDatasetCSVTransportAware(t *testing.T) {
	input := "transport," + hilPayloadHeader + "\n" +
		"hid-v3,1,1000,50,2000,2100,3000,stroke,3,0\n" +
		"hid-v3,2,4000,60,5000,5200,6000,stroke,4,1\n"

	dataset, err := ParseDatasetCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseDatasetCSV: %v", err)
	}
	if dataset.Transport != TransportHIDV3 || len(dataset.Samples) != 2 {
		t.Fatalf("unexpected dataset: %+v", dataset)
	}
}

func TestParseDatasetCSVKeepsLegacyCompatibility(t *testing.T) {
	input := hilPayloadHeader + "\n" +
		"1,1000,50,2000,2100,3000,stroke,3,0\n"
	dataset, err := ParseDatasetCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseDatasetCSV: %v", err)
	}
	if dataset.Transport != TransportLegacy || len(dataset.Samples) != 1 {
		t.Fatalf("unexpected legacy dataset: %+v", dataset)
	}
}

func TestParseDatasetCSVRejectsMixedOrUnknownTransport(t *testing.T) {
	header := "transport," + hilPayloadHeader + "\n"
	for _, body := range []string{
		"cdc-v2,1,1000,50,2000,2100,3000,stroke,3,0\nhid-v3,2,4000,60,5000,5200,6000,stroke,4,0\n",
		"bluetooth,1,1000,50,2000,2100,3000,stroke,3,0\n",
	} {
		if _, err := ParseDatasetCSV(strings.NewReader(header + body)); err == nil {
			t.Fatalf("expected transport validation failure for %q", body)
		}
	}
}
