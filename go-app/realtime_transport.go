package main

import (
	"fmt"
	"os"
	"strings"

	"hapticpad-go-app/connection"
	"hapticpad-go-app/device"
	"hapticpad-go-app/hidv3"
	"hapticpad-go-app/telemetry"
)

const realtimeTransportEnv = "KEYBOARDAZ_REALTIME_TRANSPORT"

// realtimeOpenFromEnvironment preserves the compatibility surface while new
// composition code injects one recorder through realtimeOpenFromEnvironmentWithRecorder.
func realtimeOpenFromEnvironment() connection.RealtimeOpenFunc {
	return realtimeOpenFromEnvironmentWithRecorder(nil)
}

func realtimeOpenFromEnvironmentWithRecorder(recorder telemetry.Recorder) connection.RealtimeOpenFunc {
	return realtimeOpenForModeWithRecorder(os.Getenv(realtimeTransportEnv), recorder)
}

func realtimeOpenForMode(mode string) connection.RealtimeOpenFunc {
	return realtimeOpenForModeWithRecorder(mode, nil)
}

func realtimeOpenForModeWithRecorder(mode string, recorder telemetry.Recorder) connection.RealtimeOpenFunc {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "cdc", "cdc-v2":
		return nil
	case "hid", "hid-v3", "raw-hid-v3":
		return func(identity device.Identity) (connection.EventSource, error) {
			return hidv3.OpenWithRecorder(identity, recorder)
		}
	default:
		return func(device.Identity) (connection.EventSource, error) {
			return nil, fmt.Errorf("unsupported %s=%q; expected cdc-v2 or hid-v3", realtimeTransportEnv, mode)
		}
	}
}
