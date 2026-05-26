package main

import (
	"context"
	"testing"
	"time"
)

func TestParseLoadRateConfig(t *testing.T) {
	cfg, err := parseLoadRateConfig(10, "10s-1m=200,1m-2m=50")
	if err != nil {
		t.Fatalf("parseLoadRateConfig failed: %v", err)
	}

	if cfg.baseRate != 10 {
		t.Fatalf("base rate = %d, want 10", cfg.baseRate)
	}
	if len(cfg.intervals) != 2 {
		t.Fatalf("interval count = %d, want 2", len(cfg.intervals))
	}
	if cfg.intervals[0].start != 10*time.Second || cfg.intervals[0].end != time.Minute || cfg.intervals[0].rate != 200 {
		t.Fatalf("unexpected first interval: %+v", cfg.intervals[0])
	}
	if cfg.intervals[1].start != time.Minute || cfg.intervals[1].end != 2*time.Minute || cfg.intervals[1].rate != 50 {
		t.Fatalf("unexpected second interval: %+v", cfg.intervals[1])
	}
}

func TestParseLoadRateConfigRejectsOverlap(t *testing.T) {
	_, err := parseLoadRateConfig(0, "10s-30s=10,20s-40s=20")
	if err == nil {
		t.Fatal("expected overlap error")
	}
}

func TestLoadRateControllerRateAt(t *testing.T) {
	cfg, err := parseLoadRateConfig(10, "10s-20s=0,30s-40s=50")
	if err != nil {
		t.Fatalf("parseLoadRateConfig failed: %v", err)
	}
	controller := newLoadRateController(cfg)

	rate, limited, pausedUntil := controller.rateAt(5 * time.Second)
	if rate != 10 || !limited || pausedUntil != 0 {
		t.Fatalf("rateAt before schedule = %d, %v, %s; want 10, true, 0", rate, limited, pausedUntil)
	}

	rate, limited, pausedUntil = controller.rateAt(15 * time.Second)
	if rate != 0 || !limited || pausedUntil != 20*time.Second {
		t.Fatalf("rateAt paused interval = %d, %v, %s; want 0, true, 20s", rate, limited, pausedUntil)
	}

	rate, limited, pausedUntil = controller.rateAt(35 * time.Second)
	if rate != 50 || !limited || pausedUntil != 40*time.Second {
		t.Fatalf("rateAt scheduled interval = %d, %v, %s; want 50, true, 40s", rate, limited, pausedUntil)
	}
}

func TestLoadRateControllerWaitHonorsContext(t *testing.T) {
	cfg, err := parseLoadRateConfig(0, "0s-1m=0")
	if err != nil {
		t.Fatalf("parseLoadRateConfig failed: %v", err)
	}
	controller := newLoadRateController(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if err := controller.wait(ctx); err == nil {
		t.Fatal("expected context timeout")
	}
}
