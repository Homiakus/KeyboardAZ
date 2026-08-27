package main

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/hilcapture"
	"hapticpad-go-app/inputtrace"
	"hapticpad-go-app/latencyreport"
	"hapticpad-go-app/transport"
)

func TestLoadResolverUsesBuiltInLayoutWhenPathEmpty(t *testing.T) {
	resolver, source, err := loadResolver("")
	if err != nil {
		t.Fatalf("loadResolver: %v", err)
	}
	if source != "built-in-default" {
		t.Fatalf("source=%q", source)
	}
	if _, err := resolver.ResolveStroke("en", 0, 0); err != nil {
		t.Fatalf("built-in resolver has no canonical stroke: %v", err)
	}
}

func TestLoadResolverRejectsMissingExplicitLayout(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-layout.json")
	if _, _, err := loadResolver(missing); err == nil {
		t.Fatal("expected missing explicit layout to fail")
	}
}

func TestWaitForHostCoverageReturnsAfterCorrelatedT3(t *testing.T) {
	var output bytes.Buffer
	observer, err := hilObserverForTest(&output)
	if err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Now()
	if err := observer.ObserveHIDV3(hidv3.Observation{
		Report: transport.ReportV3{
			Type:             transport.EventStroke,
			Language:         transport.LanguageEnglish,
			ButtonOrAction:   0,
			Sequence:         1,
			EventTimestampUS: 10,
		},
		HostReceivedAt: receivedAt,
	}); err != nil {
		t.Fatal(err)
	}
	observer.ObserveSendInput(inputtrace.SendInputObservation{
		Trace:    inputtrace.Trace{Transport: latencyreport.TransportHIDV3, Sequence: 1},
		CalledAt: receivedAt.Add(200 * time.Microsecond),
		Success:  true,
	})
	if err := waitForHostCoverage(context.Background(), observer, 50*time.Millisecond); err != nil {
		t.Fatalf("waitForHostCoverage: %v", err)
	}
}

func TestWaitForHostCoverageTimesOutWhenT3Missing(t *testing.T) {
	var output bytes.Buffer
	observer, err := hilObserverForTest(&output)
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.ObserveHIDV3(hidv3.Observation{
		Report: transport.ReportV3{
			Type:             transport.EventTap,
			Language:         transport.LanguageEnglish,
			ButtonOrAction:   uint8(transport.TapSpace),
			Sequence:         2,
			EventTimestampUS: 20,
		},
		HostReceivedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := waitForHostCoverage(context.Background(), observer, 5*time.Millisecond); err == nil {
		t.Fatal("expected missing T3 timeout")
	}
}

func hilObserverForTest(output *bytes.Buffer) (*hilcapture.HIDV3CSVObserver, error) {
	return hilcapture.NewHIDV3CSVObserver(output)
}
