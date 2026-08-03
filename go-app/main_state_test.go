package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"hapticpad-go-app/config"
	"hapticpad-go-app/textinput"
)

func TestAppStateSnapshotConcurrency(t *testing.T) {
	layoutConfig := textinput.DefaultLayoutConfig()
	resolver, err := textinput.NewResolver(layoutConfig)
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}

	app := &App{
		keymap:               config.DefaultKeymap(),
		resolver:             resolver,
		layoutConfig:         layoutConfig,
		layoutDraft:          textinput.CloneLayout(layoutConfig),
		maxHistory:           50,
		history:              make([]HistoryEntry, 0, 50),
		currentLanguage:      "en",
		currentMode:          "letters",
		messageProcessorStop: make(chan bool),
		messageProcessorDone: make(chan bool, 1),
	}

	var wg sync.WaitGroup

	// Concurrently append history
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				app.appendHistory(HistoryEntry{
					Type:    "stroke",
					Buttons: []int{id % 22},
					Details: fmt.Sprintf("goroutine %d iteration %d", id, j),
				})
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Concurrently update status state
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				app.mu.Lock()
				app.connected = (j%2 == 0)
				app.currentLanguage = "en"
				app.activeButtonsMask = uint32(1 << (id % 22))
				app.errorMsg = fmt.Sprintf("error %d", j)
				app.mu.Unlock()
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Concurrently read state snapshots
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				snap := app.SnapshotState()
				_ = snap.Connected
				_ = len(snap.History)
				_ = snap.ActiveButtonsMask
				_ = snap.ErrorMsg
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	snap := app.SnapshotState()
	if len(snap.History) > 50 {
		t.Errorf("expected max history 50, got %d", len(snap.History))
	}
}
