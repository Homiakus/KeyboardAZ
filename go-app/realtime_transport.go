package main

import (
	"fmt"
	"os"
	"strings"

	"hapticpad-go-app/connection"
	"hapticpad-go-app/device"
	"hapticpad-go-app/hidv3"
)

const realtimeTransportEnv = "KEYBOARDAZ_REALTIME_TRANSPORT"

// realtimeOpenFromEnvironment keeps CDC v2 as the production default. Raw HID
// v3 is enabled only by an explicit environment value so HIL/A-B runs can opt
// in without changing normal user behavior.
func realtimeOpenFromEnvironment() connection.RealtimeOpenFunc {
	return realtimeOpenForMode(os.Getenv(realtimeTransportEnv))
}

func realtimeOpenForMode(mode string) connection.RealtimeOpenFunc {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "cdc", "cdc-v2":
		return nil
	case "hid", "hid-v3", "raw-hid-v3":
		return func(identity device.Identity) (connection.EventSource, error) {
			return hidv3.Open(identity)
		}
	default:
		return func(device.Identity) (connection.EventSource, error) {
			return nil, fmt.Errorf("unsupported %s=%q; expected cdc-v2 or hid-v3", realtimeTransportEnv, mode)
		}
	}
}
