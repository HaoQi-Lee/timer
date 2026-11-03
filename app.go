package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"timer/internal/goods"
	"timer/internal/salary"
	"timer/internal/schedule"
	"timer/internal/settings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// rotationInterval 是物品提示每隔多久换一个。
const rotationInterval = 8 * time.Second

// tickInterval 是状态推送给前端的频率。
const tickInterval = 500 * time.Millisecond

// Snapshot 是推送给前端的一帧完整状态，前端只负责照着画。
type Snapshot struct {
	Date        string       `json:"date"`
	Clock       string       `json:"clock"`
	Weekday     string       `json:"weekday"`
	Status      string       `json:"status"`
	StatusTone  string       `json:"statusTone"`
	UntilOff    string       `json:"untilOff"`
	UntilOffSec int          `json:"untilOffSec"`
	UntilStart  string       `json:"untilStart"`
	Worked      string       `json:"worked"`
	TotalWork   string       `json:"totalWork"`
	Progress    float64      `json:"progress"`
	Earned      float64      `json:"earned"`
	DailyRate   float64      `json:"dailyRate"`
	HourlyRate  float64      `json:"hourlyRate"`
	MinuteRate  float64      `json:"minuteRate"`
	HintEmoji   string       `json:"hintEmoji"`
	HintItem    string       `json:"hintItem"`
	HintPrice   float64      `json:"hintPrice"`
	HintMinutes int          `json:"hintMinutes"`
	HintText    string       `json:"hintText"`
	Goods       []goods.Item `json:"goods"`
}

// App 是绑定给前端的门面，内部持有一切业务状态。
type App struct {
	ctx context.Context

	mu       sync.RWMutex
	cfg      settings.Settings
	sched    schedule.Config
	goods    []goods.Item
	rotIndex int
	rotAt    time.Time
	seed     int64
}

// NewApp 读取配置并准备好内部状态。
func NewApp() *App {
	cfg, err := settings.Load()
	if err != nil {
		fmt.Println("读取配置失败，改用默认配置:", err)
	}
	a := &App{cfg: cfg}
	a.rebuild(time.Now())
	return a
}

// rebuild 依据配置重算排班与物品轮播顺序。
func (a *App) rebuild(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sched = a.cfg.Schedule()
	a.goods = goods.Shuffled(a.cfg.AllGoods(), daySeed(now))
	a.rotIndex = 0
	a.rotAt = now
	a.seed = daySeed(now)
}

// startup 在窗口启动后开始定时推送状态。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for range ticker.C {
			a.tick()
		}
	}()
}

// startupWith 在常规启动之外，允许用命令行参数让应用一启动就打开设置面板。
func (a *App) startupWith(openSettings bool) func(ctx context.Context) {
	return func(ctx context.Context) {
		a.startup(ctx)
		if !openSettings {
			return
		}
		go func() {
			time.Sleep(800 * time.Millisecond)
			a.OpenSettings()
		}()
	}
}

// domReady 在前端就绪后摆好窗口位置，并重新确认置顶状态。
func (a *App) domReady(ctx context.Context) {
	a.mu.RLock()
	x, y := a.cfg.WindowX, a.cfg.WindowY
	top := a.cfg.AlwaysOnTop
	a.mu.RUnlock()

	if !a.restorePosition(ctx, x, y) {
		runtime.WindowCenter(ctx)
	}

	// Wails 摆窗口用的是 SetWindowPos(HWND_TOP)，这会把 WS_EX_TOPMOST 抹掉，
	// 所以摆完位置必须再置顶一次。
	if top {
		runtime.WindowSetAlwaysOnTop(ctx, true)
	}
}

// restorePosition 判断记住的窗口位置还能不能用。
//
// 位置要是跑到所有屏幕外面去，窗口就再也拖不回来了，所以这种情况直接返回 false，
// 让调用方改成居中显示。
func (a *App) restorePosition(ctx context.Context, x, y int) bool {
	if x == 0 && y == 0 {
		return false
	}

	width, height := 1920, 1080
	if screens, err := runtime.ScreenGetAll(ctx); err == nil {
		for _, s := range screens {
			if s.IsPrimary {
				width, height = s.Size.Width, s.Size.Height
				break
			}
		}
	}

	// 允许落在主屏左右各一屏的范围内：多屏时副屏可能在主屏左边（x 为负）。
	if x < -width || x >= width || y < 0 || y >= height {
		return false
	}
	runtime.WindowSetPosition(ctx, x, y)
	return true
}

// beforeClose 在窗口关闭前记住窗口位置。
func (a *App) beforeClose(ctx context.Context) bool {
	a.saveWindowPosition(ctx)
	return false
}

// saveWindowPosition 把当前窗口位置写进配置。
func (a *App) saveWindowPosition(ctx context.Context) {
	if ctx == nil {
		return
	}
	x, y := runtime.WindowGetPosition(ctx)
	a.mu.Lock()
	a.cfg.WindowX, a.cfg.WindowY = x, y
	cfg := a.cfg
	a.mu.Unlock()
	_ = settings.Save(cfg)
}

// tick 推进物品轮播并把最新状态推给前端。
func (a *App) tick() {
	now := time.Now()

	a.mu.Lock()
	if daySeed(now) != a.seed {
		// 跨天了，重新洗牌
		a.mu.Unlock()
		a.rebuild(now)
		a.mu.Lock()
	} else if now.Sub(a.rotAt) >= rotationInterval && len(a.goods) > 0 {
		a.rotIndex = (a.rotIndex + 1) % len(a.goods)
		a.rotAt = now
	}
	a.mu.Unlock()

	runtime.EventsEmit(a.ctx, "tick", a.snapshot(now))
}

// snapshot 计算某一时刻的完整状态，不改变任何内部状态。
func (a *App) snapshot(now time.Time) Snapshot {
	a.mu.RLock()
	cfg := a.cfg
	sched := a.sched
	list := a.goods
	idx := a.rotIndex
	a.mu.RUnlock()

	worked := sched.WorkedMinutes(now)
	total := sched.TotalWorkMinutes()
	rates := salary.FromAnnualSalary(cfg.AnnualSalary, total)
	earned := rates.Earned(worked)

	progress := 0.0
	if total > 0 {
		progress = float64(worked) / float64(total)
	}

	status := sched.StatusAt(now)
	untilOff := schedule.Until(now, sched.WorkEnd)

	s := Snapshot{
		Date:        now.Format("2006年01月02日"),
		Clock:       now.Format("15:04:05"),
		Weekday:     weekdayCN(now),
		Status:      string(status),
		StatusTone:  toneOf(status),
		UntilOff:    humanHMS(untilOff),
		UntilOffSec: int(untilOff.Seconds()),
		UntilStart:  untilStartText(now, sched, status),
		Worked:      humanMinutes(worked),
		TotalWork:   humanMinutes(total),
		Progress:    progress,
		Earned:      earned,
		DailyRate:   rates.DailyRate,
		HourlyRate:  rates.HourlyRate,
		MinuteRate:  rates.MinuteRate,
		Goods:       list,
	}

	if len(list) > 0 {
		item := list[idx%len(list)]
		s.HintEmoji = item.Emoji
		s.HintItem = item.Name
		s.HintPrice = item.Price
		s.HintMinutes = rates.MinutesToEarn(item.Price)
		s.HintText = hintText(status, item, earned, s.HintMinutes, rates.MinuteRate)
	}

	return s
}

// Snapshot 是给前端主动拉取当前状态用的绑定。
func (a *App) Snapshot() Snapshot {
	return a.snapshot(time.Now())
}

// GetSettings 返回当前配置。
func (a *App) GetSettings() settings.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// SaveSettings 保存配置并立即生效。
func (a *App) SaveSettings(cfg settings.Settings) error {
	cfg = cfg.Normalize()
	if err := settings.Save(cfg); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.rebuild(time.Now())
	return nil
}

// SetAlwaysOnTop 切换窗口置顶并记住这个选择。
func (a *App) SetAlwaysOnTop(on bool) error {
	a.mu.Lock()
	a.cfg.AlwaysOnTop = on
	cfg := a.cfg
	a.mu.Unlock()

	if err := settings.Save(cfg); err != nil {
		return err
	}
	if a.ctx != nil {
		runtime.WindowSetAlwaysOnTop(a.ctx, on)
	}
	return nil
}

// AlwaysOnTop 返回窗口当前是否置顶。
func (a *App) AlwaysOnTop() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.AlwaysOnTop
}

// OpenSettings 让前端弹出设置面板。
func (a *App) OpenSettings() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "open-settings")
	}
}

// Quit 关闭计时器。
func (a *App) Quit() {
	if a.ctx != nil {
		a.saveWindowPosition(a.ctx)
		runtime.Quit(a.ctx)
	}
}

// hintText 生成"再过多久能赚回某件东西"这句话。
func hintText(status schedule.Status, item goods.Item, earned float64, minutes int, minuteRate float64) string {
	switch status {
	case schedule.StatusOffWork:
		return "今天已经下班啦，明天继续 💪"
	case schedule.StatusLunch:
		return "午休中，赚钱速度暂停，稍后再算 ☕️"
	}
	if minuteRate <= 0 {
		return "设置年薪后，这里会告诉你还要多久能赚回来 🎯"
	}
	if earned >= item.Price {
		return fmt.Sprintf("%s 已经赚回来了 🎉", item.Name)
	}
	if status == schedule.StatusNotStarted {
		return fmt.Sprintf("上班后再过 %s，%s 就赚回来了", humanMinutes(minutes), item.Name)
	}
	return fmt.Sprintf("再过 %s，%s 就赚回来了", humanMinutes(minutes), item.Name)
}

// untilStartText 生成"距离上班还有多久"。
func untilStartText(now time.Time, sched schedule.Config, status schedule.Status) string {
	if status == schedule.StatusOffWork {
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location()).
			Add(time.Duration(sched.WorkStart) * time.Minute)
		return "距明天上班还有 " + humanMinutes(int(tomorrow.Sub(now).Minutes()))
	}
	if status == schedule.StatusWorking {
		return ""
	}
	return "距上班还有 " + humanMinutes(int(schedule.Until(now, sched.WorkStart).Minutes()))
}

// toneOf 把状态映射成前端用的色调标识。
func toneOf(s schedule.Status) string {
	switch s {
	case schedule.StatusWorking:
		return "working"
	case schedule.StatusLunch:
		return "lunch"
	case schedule.StatusOffWork:
		return "off"
	default:
		return "idle"
	}
}

// weekdayCN 返回中文星期。
func weekdayCN(t time.Time) string {
	names := [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
	return names[int(t.Weekday())]
}

// humanMinutes 把分钟数说成人话。
func humanMinutes(m int) string {
	if m <= 0 {
		return "0 分"
	}
	h, mm := m/60, m%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d 分", mm)
	case mm == 0:
		return fmt.Sprintf("%d 小时", h)
	default:
		return fmt.Sprintf("%d 小时 %d 分", h, mm)
	}
}

// humanHMS 把时长说成人话，精确到秒。
func humanHMS(d time.Duration) string {
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d 小时 %02d 分 %02d 秒", h, m, s)
	}
	return fmt.Sprintf("%02d 分 %02d 秒", m, s)
}

// daySeed 把日期变成一个稳定的整数，用作当天轮播顺序的随机种子。
func daySeed(t time.Time) int64 {
	return int64(t.Year())*10000 + int64(t.Month())*100 + int64(t.Day())
}
