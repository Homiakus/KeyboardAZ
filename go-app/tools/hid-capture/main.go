package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"hapticpad-go-app/device"
	"hapticpad-go-app/handler"
	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/hilcapture"
	"hapticpad-go-app/textinput"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("KeyboardAZ-hid-capture", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	vid := flags.String("vid", "", "USB VID of the KeyboardAZ device (hex, for example 2E8A)")
	pid := flags.String("pid", "", "USB PID of the KeyboardAZ device (hex)")
	serialNumber := flags.String("serial", "", "USB serial number; strongly recommended when multiple devices are attached")
	outputPath := flags.String("output", "hil-hid-v3.csv", "new HIL CSV output path")
	sampleLimit := flags.Int("samples", 10000, "number of valid HID-v3 semantic reports to capture")
	overwrite := flags.Bool("overwrite", false, "replace an existing output file")
	sendInput := flags.Bool("sendinput", false, "resolve stroke/tap events and inject them through the Windows realtime SendInput path to capture T3")
	layoutPath := flags.String("layout", "", "layout-v2.json used for -sendinput; empty uses the built-in canonical layout")
	drainTimeout := flags.Duration("drain-timeout", 5*time.Second, "maximum time to wait for all expected SendInput T3 observations")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*vid) == "" || strings.TrimSpace(*pid) == "" {
		return fmt.Errorf("-vid and -pid are required; automatic selection of an arbitrary HID interface is intentionally disabled")
	}
	if *sampleLimit <= 0 {
		return fmt.Errorf("-samples must be > 0")
	}
	if strings.TrimSpace(*outputPath) == "" {
		return fmt.Errorf("-output must not be empty")
	}
	if *drainTimeout <= 0 {
		return fmt.Errorf("-drain-timeout must be > 0")
	}
	if *sendInput && runtime.GOOS != "windows" {
		return fmt.Errorf("-sendinput HIL mode requires Windows; current platform is %s", runtime.GOOS)
	}

	resolver, layoutSource, err := loadResolver(*layoutPath)
	if err != nil {
		return err
	}

	fileFlags := os.O_CREATE | os.O_WRONLY
	if *overwrite {
		fileFlags |= os.O_TRUNC
	} else {
		fileFlags |= os.O_EXCL
	}
	output, err := os.OpenFile(*outputPath, fileFlags, 0o644)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer output.Close()

	observer, err := hilcapture.NewHIDV3CSVObserverWithLimit(output, *sampleLimit)
	if err != nil {
		return fmt.Errorf("create HIL observer: %w", err)
	}
	success := false
	defer func() {
		if success {
			return
		}
		_ = observer.Flush()
		_ = output.Sync()
		fmt.Fprintf(os.Stderr, "capture did not complete; diagnostic partial dataset may remain at %s\n", *outputPath)
	}()

	var inputHandler *handler.Handler
	if *sendInput {
		inputHandler = handler.NewHandlerWithOptions(nil, handler.HandlerOptions{SendInputObserver: observer})
		defer inputHandler.Close()
		fmt.Fprintf(os.Stderr, "HIL SendInput mode enabled: resolved keyboard input will be injected into the currently focused target window; layout=%s\n", layoutSource)
	}

	reference := device.Identity{
		VID:          *vid,
		PID:          *pid,
		SerialNumber: *serialNumber,
	}.Normalized()
	reader, err := hidv3.OpenWithOptions(reference, hidv3.OpenOptions{Observer: observer})
	if err != nil {
		return fmt.Errorf("open Raw HID v3: %w", err)
	}
	defer reader.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	captured := 0
	messages := reader.Messages()
	errorsCh := reader.Errors()
	for captured < *sampleLimit {
		select {
		case <-ctx.Done():
			return fmt.Errorf("capture interrupted after %d/%d reports", captured, *sampleLimit)
		case readErr, ok := <-errorsCh:
			if !ok {
				errorsCh = nil
				continue
			}
			if errors.Is(readErr, hilcapture.ErrCaptureLimitReached) && observer.Stats().Captured >= *sampleLimit {
				// The source-side hard limit may win the race against draining the
				// final buffered semantic message. Keep draining messages only.
				errorsCh = nil
				continue
			}
			if readErr != nil {
				return fmt.Errorf("Raw HID capture: %w", readErr)
			}
		case event, ok := <-messages:
			if !ok {
				return fmt.Errorf("Raw HID event stream closed after %d/%d reports", captured, *sampleLimit)
			}
			captured++
			if *sendInput {
				if _, err := hilcapture.DispatchHIDV3Event(event, resolver, inputHandler); err != nil {
					return fmt.Errorf("dispatch HID sequence %d: %w", event.Sequence, err)
				}
			}
		}
		if err := observer.Err(); err != nil {
			return fmt.Errorf("HIL correlation: %w", err)
		}
	}

	// Stop accepting new transport reports before validating the bounded series.
	_ = reader.Close()

	if *sendInput {
		if err := waitForHostCoverage(ctx, observer, *drainTimeout); err != nil {
			return err
		}
	}
	if err := observer.Err(); err != nil {
		return fmt.Errorf("HIL correlation: %w", err)
	}
	stats := observer.Stats()
	if stats.Captured != *sampleLimit {
		return fmt.Errorf("captured reports=%d, want exactly %d", stats.Captured, *sampleLimit)
	}
	if *sendInput {
		if stats.SendInputObserved != stats.HostTimingExpected {
			return fmt.Errorf("SendInput T3 coverage=%d/%d actionable events", stats.SendInputObserved, stats.HostTimingExpected)
		}
		if stats.SendInputFailures != 0 {
			return fmt.Errorf("SendInput failures=%d", stats.SendInputFailures)
		}
	}

	if err := observer.Flush(); err != nil {
		return fmt.Errorf("flush HIL dataset: %w", err)
	}
	if err := output.Sync(); err != nil {
		return fmt.Errorf("sync HIL dataset: %w", err)
	}
	success = true

	stats = observer.Stats()
	fmt.Printf("captured %d HID-v3 reports to %s\n", stats.Captured, *outputPath)
	if *sendInput {
		fmt.Printf("host timing coverage: %d/%d actionable events; SendInput failures=%d\n", stats.SendInputObserved, stats.HostTimingExpected, stats.SendInputFailures)
		fmt.Println("analyze with tools/latency using the default -require-host-timing=true gate")
	} else {
		fmt.Println("capture-only mode: T3 is intentionally absent; analyze with -require-host-timing=false, or rerun with -sendinput for real T2->T3 timing")
	}
	return nil
}

func loadResolver(layoutPath string) (*textinput.Resolver, string, error) {
	path := strings.TrimSpace(layoutPath)
	layout := textinput.DefaultLayoutConfig()
	source := "built-in-default"
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return nil, "", fmt.Errorf("layout %s: %w", path, err)
		}
		loaded, err := textinput.LoadLayout(path)
		if err != nil {
			return nil, "", fmt.Errorf("load layout %s: %w", path, err)
		}
		layout = loaded
		source = path
	}
	resolver, err := textinput.NewResolver(layout)
	if err != nil {
		return nil, "", fmt.Errorf("compile layout %s: %w", source, err)
	}
	return resolver, source, nil
}

func waitForHostCoverage(ctx context.Context, observer *hilcapture.HIDV3CSVObserver, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if err := observer.Err(); err != nil {
			return fmt.Errorf("HIL correlation: %w", err)
		}
		stats := observer.Stats()
		if stats.SendInputObserved >= stats.HostTimingExpected {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupted while waiting for SendInput T3 coverage=%d/%d", stats.SendInputObserved, stats.HostTimingExpected)
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for SendInput T3 coverage=%d/%d", stats.SendInputObserved, stats.HostTimingExpected)
		case <-ticker.C:
		}
	}
}
