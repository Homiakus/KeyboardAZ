package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"hapticpad-go-app/device"
	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/hilcapture"
)

func main() {
	vid := flag.String("vid", "", "USB VID of the KeyboardAZ device (hex, for example 2E8A)")
	pid := flag.String("pid", "", "USB PID of the KeyboardAZ device (hex)")
	serialNumber := flag.String("serial", "", "USB serial number; strongly recommended when multiple devices are attached")
	outputPath := flag.String("output", "hil-hid-v3.csv", "new HIL CSV output path")
	sampleLimit := flag.Int("samples", 10000, "number of valid HID-v3 semantic reports to capture")
	overwrite := flag.Bool("overwrite", false, "replace an existing output file")
	flag.Parse()

	if strings.TrimSpace(*vid) == "" || strings.TrimSpace(*pid) == "" {
		failUsage("-vid and -pid are required; automatic selection of an arbitrary HID interface is intentionally disabled")
	}
	if *sampleLimit <= 0 {
		failUsage("-samples must be > 0")
	}
	if strings.TrimSpace(*outputPath) == "" {
		failUsage("-output must not be empty")
	}

	flags := os.O_CREATE | os.O_WRONLY
	if *overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	output, err := os.OpenFile(*outputPath, flags, 0o644)
	if err != nil {
		fatalf("open output: %v", err)
	}
	success := false
	defer func() {
		_ = output.Close()
		if !success {
			fmt.Fprintf(os.Stderr, "capture did not complete; partial dataset remains at %s\n", *outputPath)
		}
	}()

	observer, err := hilcapture.NewHIDV3CSVObserver(output)
	if err != nil {
		fatalf("create HIL observer: %v", err)
	}
	reference := device.Identity{
		VID:          *vid,
		PID:          *pid,
		SerialNumber: *serialNumber,
	}.Normalized()
	reader, err := hidv3.OpenWithOptions(reference, hidv3.OpenOptions{Observer: observer})
	if err != nil {
		fatalf("open Raw HID v3: %v", err)
	}
	defer reader.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	captured := 0
	for captured < *sampleLimit {
		select {
		case <-ctx.Done():
			if err := observer.Flush(); err != nil {
				fatalf("flush interrupted capture: %v", err)
			}
			fatalf("capture interrupted after %d/%d samples", captured, *sampleLimit)
		case err, ok := <-reader.Errors():
			if !ok {
				fatalf("Raw HID error stream closed after %d/%d samples", captured, *sampleLimit)
			}
			if err != nil {
				fatalf("Raw HID capture: %v", err)
			}
		case _, ok := <-reader.Messages():
			if !ok {
				fatalf("Raw HID event stream closed after %d/%d samples", captured, *sampleLimit)
			}
			captured++
		}
	}

	if err := observer.Flush(); err != nil {
		fatalf("flush HIL dataset: %v", err)
	}
	if err := output.Sync(); err != nil {
		fatalf("sync HIL dataset: %v", err)
	}
	success = true
	fmt.Printf("captured %d HID-v3 reports to %s\n", captured, *outputPath)
	fmt.Println("analyze sequence integrity with tools/latency using -require-host-timing=false; T3 SendInput correlation is intentionally not fabricated by this capture tool")
}

func failUsage(message string) {
	fmt.Fprintln(os.Stderr, message)
	flag.Usage()
	os.Exit(2)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
