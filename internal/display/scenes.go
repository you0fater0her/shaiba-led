package display

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"ledstrip/internal/led"
)

const (
	scrollDelay        = 700 * time.Millisecond // пауза скролла, как в Python
	weatherHold        = 5 * time.Second
	slotsResultHold    = 2 * time.Second
	rainbowIntro       = 5 * time.Second
	rainbowFrame       = 30 * time.Millisecond
	rainbowCelebration = 15 * time.Minute // было, похоже, бесконечно — ограничили
)

// showTime — HHMM на ленте.
func showTime(ctx context.Context, ctl *led.Controller, now time.Time, fast bool) error {
	return ctl.ShowString(now.Format("1504"), !fast, ctx)
}

// showStaticText — до 4 символов статично, с удержанием hold.
func showStaticText(ctx context.Context, ctl *led.Controller, s string, hold time.Duration) error {
	if err := ctl.ShowString(s, false, ctx); err != nil {
		return err
	}
	return sleep(ctx, hold)
}

// showScrollText — бегущая строка (padded, как в Python show_text).
func showScrollText(ctx context.Context, ctl *led.Controller, text string, delay time.Duration) error {
	padded := "    " + text + "    "
	runes := []rune(padded)
	for i := 0; i+4 <= len(runes); i++ {
		if err := ctl.ShowString(string(runes[i:i+4]), false, ctx); err != nil {
			return err
		}
		if err := sleep(ctx, delay); err != nil {
			return err
		}
	}
	return nil
}

// countdownString — формат 4 символа:
// дни>0: "5d03", часы>0: "03h25", иначе "mmss".
func countdownString(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	days := total / 86400
	hrs := total % 86400 / 3600
	min := total % 3600 / 60
	sec := total % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%02dd%02d", days%100, hrs)
	case hrs > 0:
		return fmt.Sprintf("%02dh%02d", hrs, min)
	default:
		return fmt.Sprintf("%02d%02d", min, sec)
	}
}

// showSlotsScene — анимация слот-машины; возвращает jackpot.
// Маппинг значения кубика и порядок барабанов: led.Reels (барабан 0 = левый).
func showSlotsScene(ctx context.Context, ctl *led.Controller, value int) (bool, error) {
	target := led.Reels(value)
	const rounds = 36
	// Левый барабан останавливается первым, правый — последним.
	lockAt := [4]int{rounds - 24, rounds - 16, rounds - 8, rounds - 2}

	cur := target
	for r := 0; r < rounds; r++ {
		for i := 0; i < 4; i++ {
			if r == lockAt[i] {
				cur[i] = target[i]
			} else if r < lockAt[i] {
				cur[i] = led.SlotNames[rand.Intn(len(led.SlotNames))]
			}
		}
		for pos := 0; pos < 4; pos++ {
			// барабан i -> позиция 3-i (позиция 0 = правая на ленте)
			if err := ctl.ShowSlotSymbol(pos, cur[3-pos], false, ctx); err != nil {
				return false, err
			}
		}
		// Разгон и замедление.
		pause := time.Duration(30+r*8) * time.Millisecond
		if err := sleep(ctx, pause); err != nil {
			return false, err
		}
	}

	if led.IsJackpot(value) {
		// Мигание всеми цветами на финале.
		for i := 0; i < 6; i++ {
			for pos := 0; pos < 4; pos++ {
				if err := ctl.ShowSlotSymbol(pos, target[3-pos], false, ctx); err != nil {
					return true, err
				}
			}
			if err := sleep(ctx, 200*time.Millisecond); err != nil {
				return true, err
			}
		}
	}
	return led.IsJackpot(value), nil
}

// rainbowCycle — волна радуги по всей ленте, ограниченная по времени
// и отменяемая shutdown'ом (исправление бесконечного цикла Python-версии).
func rainbowCycle(ctx context.Context, ctl *led.Controller, dur time.Duration) error {
	deadline := time.NewTimer(dur)
	defer deadline.Stop()
	offset := 0
	for {
		ctl.FillRainbow(offset)
		offset = (offset + 2) & 255
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return nil
		default:
		}
		if err := sleep(ctx, rainbowFrame); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}