package display

import (
	"context"
	"log"
	"sync"
	"time"

	"ledstrip/internal/led"
	"ledstrip/internal/services"
	"ledstrip/internal/storage"
)

type Mode int

const (
	ModeMain Mode = iota
	ModeMainNext // резерв, не используется
	ModeUser
	ModeFight
)

const (
	tickInterval   = 100 * time.Millisecond
	finalMinute    = time.Minute
	cmdQueueSize   = 32
)

type cmdKind int

const (
	cmdShowText cmdKind = iota
	cmdShowSlots
	cmdShowTemp
	cmdRainbowIntro
	cmdSetNewYear
)

type command struct {
	kind cmdKind
	s    string // текст
	n    int    // значение кубика
	t    time.Time
}

// Engine — диспетчер дисплея. Единственная горутина, выполняющая сцены:
// порядок команд строго очередной, гонок состояния нет.
// Хендлеры бота шлют команды в cmdq; мгновенные изменения (цвет, радуга)
// идут напрямую в потокобезопасный Controller — как в Python, где хендлеры
// трогали led_controller напрямую.
type Engine struct {
	ctl     *led.Controller
	weather *services.Weather
	store   *storage.Store

	cmdq chan command

	mu      sync.Mutex
	mode    Mode
	newYear time.Time
	anim    bool // идёт анимация (текст/температура/слоты)
	slots   bool // слоты заняты
}

func NewEngine(ctl *led.Controller, w *services.Weather, s *storage.Store, newYear time.Time) *Engine {
	return &Engine{
		ctl:     ctl,
		weather: w,
		store:   s,
		cmdq:    make(chan command, cmdQueueSize),
		mode:    ModeMain,
		newYear: newYear,
	}
}

// --- API для хендлеров бота ---

func (e *Engine) Mode() Mode {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode
}

func (e *Engine) SetMode(m Mode) {
	e.mu.Lock()
	e.mode = m
	e.mu.Unlock()
}

func (e *Engine) NewYear() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.newYear
}

func (e *Engine) Busy() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.anim || e.slots
}

func (e *Engine) SlotsActive() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.slots
}

// CountdownProtected — финальная минута отсчёта: не-админы не управляют.
func (e *Engine) CountdownProtected() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.mode != ModeFight {
		return false
	}
	d := time.Until(e.newYear)
	return d > 0 && d <= finalMinute
}

func (e *Engine) enqueue(cmd command) bool {
	select {
	case e.cmdq <- cmd:
		return true
	default:
		return false // очередь полна — «подожди пару секунд»
	}
}

// ShowText — бегущая строка (вызов уже проверил Busy/rate-limit).
func (e *Engine) ShowText(text string) bool {
	return e.enqueue(command{kind: cmdShowText, s: text})
}

func (e *Engine) ShowTemperature() bool {
	return e.enqueue(command{kind: cmdShowTemp})
}

// RequestSlots атомарно резервирует слоты; false = уже крутятся.
func (e *Engine) RequestSlots(value int) bool {
	e.mu.Lock()
	if e.slots {
		e.mu.Unlock()
		return false
	}
	e.slots = true
	e.mu.Unlock()

	if !e.enqueue(command{kind: cmdShowSlots, n: value}) {
		e.mu.Lock()
		e.slots = false
		e.mu.Unlock()
		return false
	}
	return true
}

// SetColor — мгновенно (обновляет и уже горящие пиксели).
func (e *Engine) SetColor(c led.RGB) {
	e.ctl.SetColor(c)
	e.ctl.RecolorLit()
}

// ToggleRainbow — мгновенно включает/выключает; 5-секундная «распаковка»
// радуги ставится в очередь.
func (e *Engine) ToggleRainbow() bool {
	on := e.ctl.ToggleRainbow()
	e.ctl.RecolorLit()
	e.enqueue(command{kind: cmdRainbowIntro})
	return on
}

func (e *Engine) SetNewYearDate(t time.Time) bool {
	return e.enqueue(command{kind: cmdSetNewYear, t: t})
}

// --- Главный цикл ---

func (e *Engine) Run(ctx context.Context) error {
	defer e.ctl.Clear() // гасим ленту при любом выходе

	tick := time.NewTicker(tickInterval)
	defer tick.Stop()

	var lastTime, lastCD string

	for {
		select {
		case <-ctx.Done():
			return nil
		case cmd := <-e.cmdq:
			e.exec(ctx, cmd)
			lastTime, lastCD = "", "" // принудительная перерисовка сцены
		case <-tick.C:
			e.tick(ctx, &lastTime, &lastCD)
		}
	}
}

func (e *Engine) tick(ctx context.Context, lastTime, lastCD *string) {
	switch e.Mode() {
	case ModeMain, ModeUser:
		now := time.Now().Format("15:04")
		if now != *lastTime {
			*lastTime = now
			e.onNewMinute(ctx, e.Mode())
		}
	case ModeFight:
		d := time.Until(e.NewYear())
		if d <= 0 {
			e.finishCountdown(ctx)
			*lastCD = ""
			return
		}
		s := countdownString(d)
		fast := d <= finalMinute // FIGHT_FAST: без анимации, апдейт каждую секунду
		if s != *lastCD {
			*lastCD = s
			if err := e.ctl.ShowString(s, !fast, ctx); err != nil {
				return
			}
		}
	}
}

// onNewMinute — погода (5с) → [USER: «С НОВЫМ ГОДОМ» в радуге] → время.
func (e *Engine) onNewMinute(ctx context.Context, mode Mode) {
	e.ensureNonBlackColor()

	if disp := e.weather.Display(); disp != "" {
		e.runAnim(func() error {
			return showStaticText(ctx, e.ctl, disp, weatherHold)
		})
	}

	if mode == ModeUser {
		e.runAnim(func() error {
			prev := e.ctl.Rainbow()
			e.ctl.SetRainbow(true)
			err := showScrollText(ctx, e.ctl, "С НОВЫМ ГОДОМ", scrollDelay)
			e.ctl.SetRainbow(prev) // возвращаем прежний режим (Python гасил принудительно)
			e.ctl.RecolorLit()
			return err
		})
	}

	e.runAnim(func() error {
		return showTime(ctx, e.ctl, time.Now(), false)
	})
}

func (e *Engine) finishCountdown(ctx context.Context) {
	e.mu.Lock()
	e.mode = ModeMain
	e.mu.Unlock()

	e.runAnim(func() error {
		return showScrollText(ctx, e.ctl, "ПУНК! С НОВЫМ ГОДОМ!!!", scrollDelay)
	})
	// Праздничная радуга по всей ленте (ограничена по времени, см. scenes.go).
	_ = rainbowCycle(ctx, e.ctl, rainbowCelebration)
}

func (e *Engine) exec(ctx context.Context, cmd command) {
	switch cmd.kind {
	case cmdShowText:
		e.runAnim(func() error {
			return showScrollText(ctx, e.ctl, cmd.s, scrollDelay)
		})
	case cmdShowTemp:
		e.runAnim(func() error {
			disp := e.weather.Display()
			if disp == "" {
				disp = "--°"
			}
			return showStaticText(ctx, e.ctl, disp, weatherHold)
		})
	case cmdShowSlots:
		defer func() {
			e.mu.Lock()
			e.slots = false
			e.mu.Unlock()
		}()
		e.runAnim(func() error {
			_, err := showSlotsScene(ctx, e.ctl, cmd.n)
			if err == nil {
				err = sleep(ctx, slotsResultHold)
			}
			return err
		})
	case cmdRainbowIntro:
		if !e.ctl.Rainbow() {
			return
		}
		// 5 секунд плавной радуги над текущим кадром (как в Python).
		deadline := time.Now().Add(rainbowIntro)
		for time.Now().Before(deadline) {
			e.ctl.RecolorLit()
			if sleep(ctx, rainbowFrame) != nil {
				return
			}
		}
	case cmdSetNewYear:
		e.mu.Lock()
		e.newYear = cmd.t
		e.mu.Unlock()
		log.Printf("new year date updated: %s", cmd.t.Format("2006-01-02 15:04:05"))
	}
}

// runAnim помечает дисплей занятым на время сцены.
func (e *Engine) runAnim(fn func() error) {
	e.mu.Lock()
	e.anim = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.anim = false
		e.mu.Unlock()
	}()
	_ = fn()
}

// ensureNonBlackColor — чёрный цвет на новой минуте заменяется случайным
// (и сохраняется), как в Python.
func (e *Engine) ensureNonBlackColor() {
	if e.ctl.Color() != led.Black {
		return
	}
	c := led.RandomColor()
	e.ctl.SetColor(c)
	e.store.SaveColor(c)
	e.ctl.RecolorLit()
}