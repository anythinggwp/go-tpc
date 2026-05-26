package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type loadRateInterval struct {
	start time.Duration
	end   time.Duration
	rate  int
}

type loadRateConfig struct {
	baseRate  int
	intervals []loadRateInterval
}

type loadRateController struct {
	cfg   loadRateConfig
	start time.Time

	mu   sync.Mutex
	next time.Time
}

func parseLoadRateConfig(baseRate int, schedule string) (loadRateConfig, error) {
	if baseRate < 0 {
		return loadRateConfig{}, fmt.Errorf("rate must be >= 0")
	}

	cfg := loadRateConfig{baseRate: baseRate}
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return cfg, nil
	}

	parts := strings.Split(schedule, ",")
	cfg.intervals = make([]loadRateInterval, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		rangeAndRate := strings.Split(part, "=")
		if len(rangeAndRate) != 2 {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule segment %q, expected start-end=rate", part)
		}

		timeRange := strings.Split(rangeAndRate[0], "-")
		if len(timeRange) != 2 {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule range %q, expected start-end", rangeAndRate[0])
		}

		start, err := time.ParseDuration(strings.TrimSpace(timeRange[0]))
		if err != nil {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule start %q: %w", timeRange[0], err)
		}
		end, err := time.ParseDuration(strings.TrimSpace(timeRange[1]))
		if err != nil {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule end %q: %w", timeRange[1], err)
		}
		if start < 0 || end <= start {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule range %q, end must be greater than start", rangeAndRate[0])
		}

		rate, err := strconv.Atoi(strings.TrimSpace(rangeAndRate[1]))
		if err != nil {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule rate %q: %w", rangeAndRate[1], err)
		}
		if rate < 0 {
			return loadRateConfig{}, fmt.Errorf("invalid rate schedule rate %d, expected >= 0", rate)
		}

		cfg.intervals = append(cfg.intervals, loadRateInterval{
			start: start,
			end:   end,
			rate:  rate,
		})
	}

	sort.Slice(cfg.intervals, func(i, j int) bool {
		return cfg.intervals[i].start < cfg.intervals[j].start
	})
	for i := 1; i < len(cfg.intervals); i++ {
		if cfg.intervals[i].start < cfg.intervals[i-1].end {
			return loadRateConfig{}, fmt.Errorf("rate schedule intervals overlap: %s-%s and %s-%s",
				cfg.intervals[i-1].start, cfg.intervals[i-1].end, cfg.intervals[i].start, cfg.intervals[i].end)
		}
	}

	return cfg, nil
}

func newLoadRateController(cfg loadRateConfig) *loadRateController {
	if cfg.baseRate == 0 && len(cfg.intervals) == 0 {
		return nil
	}
	return &loadRateController{
		cfg:   cfg,
		start: time.Now(),
	}
}

func (c *loadRateController) wait(ctx context.Context) error {
	for {
		rate, limited, pausedUntil := c.rateAt(time.Since(c.start))
		if !limited {
			return nil
		}
		if rate == 0 {
			wait := pausedUntil - time.Since(c.start)
			if wait <= 0 {
				continue
			}
			return sleepWithContext(ctx, wait)
		}

		interval := time.Second / time.Duration(rate)
		if interval <= 0 {
			return nil
		}

		now := time.Now()
		c.mu.Lock()
		if c.next.Before(now) {
			c.next = now
		}
		wait := c.next.Sub(now)
		c.next = c.next.Add(interval)
		c.mu.Unlock()

		if wait <= 0 {
			return nil
		}
		return sleepWithContext(ctx, wait)
	}
}

func (c *loadRateController) rateAt(elapsed time.Duration) (rate int, limited bool, pausedUntil time.Duration) {
	for _, interval := range c.cfg.intervals {
		if elapsed >= interval.start && elapsed < interval.end {
			return interval.rate, true, interval.end
		}
	}
	if c.cfg.baseRate > 0 {
		return c.cfg.baseRate, true, 0
	}
	return 0, false, 0
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
