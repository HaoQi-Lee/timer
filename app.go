package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"time"

	"timer/internal/goods"
	"timer/internal/salary"
	"timer/internal/schedule"
	"timer/internal/settings"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// tickInterval 是状态推送给前端的频率。
const tickInterval = 500 * time.Millisecond

// mainWindowTitle 主窗口标题，多屏摆位时按它找窗口句柄。
const mainWindowTitle = "上班计时器"

// Snapshot 是推送给前端的一帧完整状态，前端只负责照着画。
//
// 只放前端真会显示的字段——它每 500ms 就序列化发一次，
// 没人读的数据（比如整个物品清单）不要塞进来。
type Snapshot struct {
	Date         string  `json:"date"`
	Clock        string  `json:"clock"`
	Weekday      string  `json:"weekday"`
	Status       string  `json:"status"`
	StatusTone   string  `json:"statusTone"`
	UntilOff     string  `json:"untilOff"`
	UntilStart   string  `json:"untilStart"`
	Worked       string  `json:"worked"`
	TotalWork    string  `json:"totalWork"`
	Progress     float64 `json:"progress"`
	Earned       float64 `json:"earned"`
	HourlyRate   float64 `json:"hourlyRate"`
	MinuteRate   float64 `json:"minuteRate"`
	HintEmoji    string  `json:"hintEmoji"`
	HintItem     string  `json:"hintItem"`
	HintPrice    float64 `json:"hintPrice"`
	HintMinutes  int     `json:"hintMinutes"`
	HintProgress float64 `json:"hintProgress"`
	HintText     string  `json:"hintText"`
	// OpenSettings 启动时要不要直接弹出设置面板。
	//
	// 用状态位而不是事件来通知，是为了避开"事件比前端注册得早"的竞态。
	OpenSettings bool `json:"openSettings"`
}

// App 是绑定给前端的门面，内部持有一切业务状态。
type App struct {
	ctx context.Context

	mu    sync.RWMutex
	cfg   settings.Settings
	sched schedule.Config
	goods []goods.Item
	// hint 是当前盯着的那件物品，挑定之后就不再换，
	// 直到它买得起为止（见 refreshHint）。
	hint *goods.Item
	rnd  *rand.Rand
	seed int64
	// openOnStart 启动后要不要自动弹出设置面板。
	openOnStart bool
	// quiet 为 true 时只算状态不发提醒：宠物进程复用本引擎，
	// 下班通知只由主进程发一次，不然会弹两份。
	quiet bool
	// lastStatus 上次 tick 时的阶段，用来识别"刚跨过下班点"。
	lastStatus schedule.Status

	// 宠物子进程。Wails v2 没有多窗口，桌面宠物靠同一个 exe
	// 再拉一个 -pet 进程实现，主进程负责它的生杀。
	petMu  sync.Mutex
	petCmd *exec.Cmd
}

// NewApp 读取配置并准备好内部状态。
func NewApp(openOnStart bool) *App {
	cfg, err := settings.Load()
	if err != nil {
		fmt.Println("读取配置失败，改用默认配置:", err)
	}
	a := &App{
		cfg:         cfg,
		rnd:         rand.New(rand.NewSource(time.Now().UnixNano())),
		openOnStart: openOnStart,
	}
	a.rebuild(time.Now())
	return a
}

// rebuild 依据配置重算排班与物品清单。
func (a *App) rebuild(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sched = a.cfg.Schedule()
	a.goods = a.cfg.AllGoods()
	a.hint = nil // 换了配置就重新挑一件
	a.seed = daySeed(now)
}

// applyConfig 替换配置并立即生效（宠物进程热加载主窗口的改动用）。
func (a *App) applyConfig(cfg settings.Settings) {
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.rebuild(time.Now())
}

// updateSettings 从磁盘读最新配置、只改指定字段后写回。
//
// 主进程和宠物进程共用一个配置文件，谁都不能拿内存里的整份副本
// 直接覆盖磁盘——否则会把另一边刚写的字段冲掉
// （典型事故：主进程保存设置，把宠物刚挪好的位置打回旧坐标）。
func updateSettings(mutate func(cfg *settings.Settings)) error {
	cfg, err := settings.Load()
	if err != nil {
		cfg = settings.Default()
	}
	mutate(&cfg)
	return settings.Save(cfg)
}

// timerSignature 提取影响计时数字段的指纹，用来判断配置变更要不要热加载。
//
// 只看计时相关字段：宠物只关心这些，位置之类的字段变了不用重算。
func timerSignature(s settings.Settings) string {
	return fmt.Sprintf("%.0f|%s|%s|%s|%s|%v",
		s.AnnualSalary, s.WorkStart, s.LunchStart, s.WorkResume, s.WorkEnd, s.CustomGoods)
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

// domReady 在前端就绪后摆好窗口位置，并重新确认置顶状态。
func (a *App) domReady(ctx context.Context) {
	a.mu.RLock()
	x, y := a.cfg.WindowX, a.cfg.WindowY
	top := a.cfg.AlwaysOnTop
	a.mu.RUnlock()

	if !placeWindow(ctx, mainWindowTitle, x, y) {
		runtime.WindowCenter(ctx)
	}

	// Wails 摆窗口用的是 SetWindowPos(HWND_TOP)，这会把 WS_EX_TOPMOST 抹掉，
	// 所以摆完位置必须再置顶一次。
	if top {
		runtime.WindowSetAlwaysOnTop(ctx, true)
	}

	// 上次开着桌面宠物的话，这次也把它拉起来。
	a.mu.RLock()
	petOn := a.cfg.PetEnabled
	a.mu.RUnlock()
	if petOn {
		if err := a.startPet(); err != nil {
			fmt.Println("启动桌面宠物失败:", err)
		}
	}
}

// placeWindow 把窗口摆到记住的屏幕坐标 (x, y)。
//
// Windows 上优先走 Win32 的绝对坐标摆位（Wails 的 WindowSetPosition
// 是"当前显示器工作区相对坐标"，而保存下来的位置是物理绝对坐标，
// 多屏时会错位）；其它平台或找不到句柄时退回 Wails。
// 返回 false 表示位置无效（比如记在副屏上、副屏拔了），调用方改居中。
func placeWindow(ctx context.Context, title string, x, y int) bool {
	if x == 0 && y == 0 {
		return false // 还没记住过位置
	}
	if nativePlaceWindow(title, x, y) {
		return true
	}
	// 兜底：Wails 拿不到各屏位置，只能按"主屏左右一屏"的启发式放行。
	screens, err := runtime.ScreenGetAll(ctx)
	if err != nil {
		return false
	}
	for _, s := range screens {
		w, h := s.Size.Width, s.Size.Height
		if x >= -w && x < w && y >= 0 && y < h {
			runtime.WindowSetPosition(ctx, x, y)
			return true
		}
	}
	return false
}

// beforeClose 在窗口关闭前记住窗口位置，并把宠物一起带走。
func (a *App) beforeClose(ctx context.Context) bool {
	a.saveWindowPosition(ctx)
	a.stopPet()
	return false
}

// saveWindowPosition 把当前窗口位置写进配置（合并写，只动位置字段）。
func (a *App) saveWindowPosition(ctx context.Context) {
	if ctx == nil {
		return
	}
	x, y := runtime.WindowGetPosition(ctx)
	a.mu.Lock()
	a.cfg.WindowX, a.cfg.WindowY = x, y
	a.mu.Unlock()
	_ = updateSettings(func(cfg *settings.Settings) {
		cfg.WindowX, cfg.WindowY = x, y
	})
}

// tick 刷新提示区状态并把最新状态推给前端。
func (a *App) tick() {
	now := time.Now()

	a.mu.RLock()
	newDay := daySeed(now) != a.seed
	a.mu.RUnlock()
	if newDay {
		// 跨天了，重新挑一件物品
		a.rebuild(now)
	}

	rates, worked, earned := a.rates(now)
	a.refreshHint(earned, rates.DailyRate)

	snap := a.snapshot(now, rates, worked, earned)
	a.maybeNotifyOffWork(schedule.Status(snap.Status), earned)
	runtime.EventsEmit(a.ctx, "tick", snap)
}

// maybeNotifyOffWork 检测"刚跨过下班点"，弹一次系统提醒。
//
// 只认上班中→已下班的跳变：启动时已经在晚上不会补弹；
// 周末没有上班中状态，自然也不会弹。
func (a *App) maybeNotifyOffWork(status schedule.Status, earned float64) {
	a.mu.Lock()
	prev := a.lastStatus
	a.lastStatus = status
	fire := !a.quiet && a.cfg.WantOffWorkNotify() &&
		prev == schedule.StatusWorking && status == schedule.StatusOffWork
	a.mu.Unlock()

	if fire {
		go nativeNotify("下班啦 🎉", fmt.Sprintf("今天已经赚 ￥%.2f，辛苦了，收工！", earned))
	}
}

// rates 计算某时刻的薪资速度、已工作分钟数与已赚到的钱。
func (a *App) rates(now time.Time) (salary.Rates, int, float64) {
	a.mu.RLock()
	cfg := a.cfg
	sched := a.sched
	a.mu.RUnlock()

	total := sched.TotalWorkMinutes()
	rates := salary.FromAnnualSalary(cfg.AnnualSalary, total)
	worked := sched.WorkedMinutes(now)
	return rates, worked, rates.Earned(worked)
}

// refreshHint 决定当前提示哪件物品。
//
// 规则：
//   - 只看今天一天赚得到的物品，也就是价格不超过日薪；
//   - 已经买得起的物品不再提示；
//   - 随机挑一件显示它的进度，挑定之后就不再换，
//     直到它买得起为止，才换下一件。
func (a *App) refreshHint(earned, dailyRate float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 当前这件还没买得起，就一直盯着它。
	if a.hint != nil && a.hint.Price > earned && a.hint.Price <= dailyRate {
		return
	}

	pool := goods.Pending(a.goods, earned, dailyRate)
	if len(pool) == 0 {
		a.hint = nil
		return
	}

	if a.rnd == nil {
		a.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	pick := pool[a.rnd.Intn(len(pool))]
	a.hint = &pick
}

// snapshot 计算某一时刻的完整状态，不改变任何内部状态。
func (a *App) snapshot(now time.Time, rates salary.Rates, worked int, earned float64) Snapshot {
	a.mu.RLock()
	sched := a.sched
	list := a.goods
	hint := a.hint
	openOnStart := a.openOnStart
	a.mu.RUnlock()

	total := sched.TotalWorkMinutes()

	progress := 0.0
	if total > 0 {
		progress = float64(worked) / float64(total)
	}

	status := sched.StatusAt(now)
	untilOff := schedule.Until(now, sched.WorkEnd)

	s := Snapshot{
		Date:         now.Format("2006年01月02日"),
		Clock:        now.Format("15:04:05"),
		Weekday:      weekdayCN(now),
		Status:       string(status),
		StatusTone:   toneOf(status),
		UntilOff:     humanHMS(untilOff),
		UntilStart:   untilStartText(now, sched, status),
		Worked:       humanMinutes(worked),
		TotalWork:    humanMinutes(total),
		Progress:     progress,
		Earned:       earned,
		HourlyRate:   rates.HourlyRate,
		MinuteRate:   rates.MinuteRate,
		OpenSettings: openOnStart,
	}

	fillHint(&s, status, rates, earned, list, hint)

	return s
}

// fillHint 填好提示区的内容。
func fillHint(s *Snapshot, status schedule.Status, rates salary.Rates, earned float64, list []goods.Item, hint *goods.Item) {
	if rates.MinuteRate <= 0 {
		s.HintEmoji = "💡"
		s.HintText = "设置年薪后，这里会告诉你还要多久能赚回来 🎯"
		return
	}

	if hint == nil {
		// 没有可提示的物品：要么今天根本赚不到清单里的东西，
		// 要么今天赚得到的都已经买得起。
		if len(goods.Reachable(list, rates.DailyRate)) == 0 {
			s.HintEmoji = "💡"
			s.HintText = "按现在的年薪，一天赚不到清单里的东西，调高年薪试试 🎯"
		} else {
			s.HintEmoji = "🎉"
			s.HintText = "今天赚得到的东西都买得起啦 🎉"
		}
		return
	}

	s.HintEmoji = hint.Emoji
	s.HintItem = hint.Name
	s.HintPrice = hint.Price
	s.HintMinutes = rates.MinutesToEarnFrom(earned, hint.Price)
	if hint.Price > 0 {
		s.HintProgress = earned / hint.Price
		if s.HintProgress > 1 {
			s.HintProgress = 1
		}
	}
	s.HintText = hintLine(status, *hint, s.HintMinutes)
}

// hintLine 生成提示区的那句话。
func hintLine(status schedule.Status, item goods.Item, minutes int) string {
	switch status {
	case schedule.StatusOffWork:
		return fmt.Sprintf("%s · 今天已经下班啦，明天继续 💪", item.Name)
	case schedule.StatusWeekend:
		return fmt.Sprintf("%s · 今天休息，不赚钱也不花钱 🛌", item.Name)
	case schedule.StatusLunch:
		return fmt.Sprintf("%s · 午休中，赚钱速度暂停 ☕️", item.Name)
	case schedule.StatusNotStarted:
		return fmt.Sprintf("%s · 上班后再过 %s 就赚回来了", item.Name, humanMinutes(minutes))
	default:
		return fmt.Sprintf("%s · 再过 %s 就赚回来了", item.Name, humanMinutes(minutes))
	}
}

// Snapshot 是给前端主动拉取当前状态用的绑定。
func (a *App) Snapshot() Snapshot {
	now := time.Now()
	rates, worked, earned := a.rates(now)
	a.refreshHint(earned, rates.DailyRate)
	return a.snapshot(now, rates, worked, earned)
}

// GetSettings 返回当前配置。
func (a *App) GetSettings() settings.Settings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// SaveSettings 保存配置并立即生效。
//
// 宠物管理的字段（位置、心情包、置顶）以磁盘上的最新值为准：
// 设置面板开着的时候宠物可能挪了位置，不能被面板打开时的旧值冲掉。
// 开机自启顺带同步到注册表。
func (a *App) SaveSettings(cfg settings.Settings) error {
	cfg = cfg.Normalize()
	if disk, err := settings.Load(); err == nil {
		cfg.PetX, cfg.PetY = disk.PetX, disk.PetY
		cfg.PetPack = disk.PetPack
		cfg.PetAlwaysOnTop = disk.PetAlwaysOnTop
	}
	if a.ctx != nil {
		cfg.WindowX, cfg.WindowY = runtime.WindowGetPosition(a.ctx)
	}
	if err := settings.Save(cfg); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.rebuild(time.Now())

	if err := applyAutoStart(cfg.AutoStart); err != nil {
		fmt.Println("同步开机自启失败:", err)
	}
	return nil
}

// SetAlwaysOnTop 切换窗口置顶并记住这个选择。
func (a *App) SetAlwaysOnTop(on bool) error {
	a.mu.Lock()
	a.cfg.AlwaysOnTop = on
	a.mu.Unlock()

	if err := updateSettings(func(cfg *settings.Settings) { cfg.AlwaysOnTop = on }); err != nil {
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

// SetTheme 切换界面主题并记住这个选择。
func (a *App) SetTheme(theme string) error {
	if theme != settings.ThemeDark && theme != settings.ThemeLight {
		return fmt.Errorf("未知主题: %q", theme)
	}
	a.mu.Lock()
	a.cfg.Theme = theme
	a.mu.Unlock()

	return updateSettings(func(cfg *settings.Settings) { cfg.Theme = theme })
}

// Theme 返回当前主题（"dark" 或 "light"）。
func (a *App) Theme() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.Theme
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

// Minimize 把窗口收进任务栏。
func (a *App) Minimize() {
	if a.ctx != nil {
		runtime.WindowMinimise(a.ctx)
	}
}

// onSecondInstance 有人想再开一个实例时走这里（单实例锁把第二个拦下并退出，
// 第一个收到这条通知）：把窗口唤起来，对方带着 -s 参数的话顺手弹设置面板。
func (a *App) onSecondInstance(data options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	for _, arg := range data.Args {
		if arg == "-s" || arg == "--settings" {
			a.OpenSettings()
			return
		}
	}
}

// PetEnabled 返回桌面宠物开关状态。
func (a *App) PetEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg.PetEnabled
}

// SetPetEnabled 开关桌面宠物并记住这个选择。
func (a *App) SetPetEnabled(on bool) error {
	a.mu.Lock()
	a.cfg.PetEnabled = on
	a.mu.Unlock()

	if err := updateSettings(func(cfg *settings.Settings) { cfg.PetEnabled = on }); err != nil {
		return err
	}
	if on {
		return a.startPet()
	}
	a.stopPet()
	return nil
}

// startPet 拉起桌宠子进程，已经在跑就不重复拉。
func (a *App) startPet() error {
	a.petMu.Lock()
	defer a.petMu.Unlock()

	if a.petCmd != nil && a.petCmd.Process != nil && a.petCmd.ProcessState == nil {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "-pet")
	if err := cmd.Start(); err != nil {
		return err
	}
	a.petCmd = cmd
	go a.watchPet(cmd)
	return nil
}

// watchPet 盯着宠物进程。它自己退出（比如右键收起）时，
// 把开关状态同步成关，并通知前端刷新界面。
func (a *App) watchPet(cmd *exec.Cmd) {
	_ = cmd.Wait()

	a.petMu.Lock()
	current := a.petCmd == cmd
	if current {
		a.petCmd = nil
	}
	a.petMu.Unlock()
	if !current {
		return // 已经被主动停掉或换成了新进程，不用管
	}

	// 只改开关字段：拿内存里的整份配置覆盖会把宠物刚存的位置冲掉。
	a.mu.Lock()
	a.cfg.PetEnabled = false
	a.mu.Unlock()
	_ = updateSettings(func(cfg *settings.Settings) { cfg.PetEnabled = false })

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "pet-state", false)
	}
}

// stopPet 停掉桌宠子进程。主进程退出时也走这里，宠物跟着一起走。
func (a *App) stopPet() {
	a.petMu.Lock()
	cmd := a.petCmd
	a.petCmd = nil
	a.petMu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	// 收尸，免得留下僵尸进程句柄；watchPet 里的 Wait 已经失效，无害。
	go func() { _ = cmd.Wait() }()
}

// untilStartText 生成"距离上班还有多久"。
//
// 下班后和周末都指向下一个工作日的上班时刻：周五晚上说"距星期一上班"，
// 不再把周六当成要上班的日子。
func untilStartText(now time.Time, sched schedule.Config, status schedule.Status) string {
	switch status {
	case schedule.StatusWorking:
		return ""
	case schedule.StatusLunch:
		// 原先这里会算出"距上班还有 0 分"（上班时刻已过），改成指向下午上班
		return "距下午上班还有 " + humanMinutes(int(schedule.Until(now, sched.WorkResume).Minutes()))
	case schedule.StatusOffWork, schedule.StatusWeekend:
		next := nextWorkdayStart(now, sched)
		return "距" + nextWorkdayLabel(now, next) + "上班还有 " + humanMinutes(int(next.Sub(now).Minutes()))
	default: // 未上班（工作日早上）
		return "距上班还有 " + humanMinutes(int(schedule.Until(now, sched.WorkStart).Minutes()))
	}
}

// nextWorkdayStart 找 now 之后的第一个工作日上班时刻。
func nextWorkdayStart(now time.Time, sched schedule.Config) time.Time {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		Add(time.Duration(sched.WorkStart) * time.Minute)
	for !schedule.IsWorkday(d) || !d.After(now) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// nextWorkdayLabel 把目标日说成人话：明天就是"明天"，隔了周末就说星期几。
func nextWorkdayLabel(now, next time.Time) string {
	tomorrow := now.AddDate(0, 0, 1)
	if next.Year() == tomorrow.Year() && next.Month() == tomorrow.Month() && next.Day() == tomorrow.Day() {
		return "明天"
	}
	return weekdayCN(next)
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
	case schedule.StatusWeekend:
		return "rest"
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

// daySeed 把日期变成一个稳定的整数，用来判断是不是换天了。
func daySeed(t time.Time) int64 {
	return int64(t.Year())*10000 + int64(t.Month())*100 + int64(t.Day())
}
