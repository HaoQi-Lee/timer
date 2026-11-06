package main

import (
	"math/rand"
	"testing"
	"time"

	"timer/internal/goods"
	"timer/internal/salary"
	"timer/internal/schedule"
	"timer/internal/settings"
)

func TestHumanMinutes(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0 分"},
		{-5, "0 分"},
		{1, "1 分"},
		{45, "45 分"},
		{60, "1 小时"},
		{83, "1 小时 23 分"},
		{420, "7 小时"},
		{217, "3 小时 37 分"},
	}
	for _, tc := range cases {
		if got := humanMinutes(tc.in); got != tc.want {
			t.Errorf("humanMinutes(%d) 应为 %q，实际 %q", tc.in, tc.want, got)
		}
	}
}

func TestHumanHMS(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "00 分 00 秒"},
		{59 * time.Second, "00 分 59 秒"},
		{90 * time.Second, "01 分 30 秒"},
		{time.Hour + 2*time.Minute + 3*time.Second, "1 小时 02 分 03 秒"},
	}
	for _, tc := range cases {
		if got := humanHMS(tc.in); got != tc.want {
			t.Errorf("humanHMS(%v) 应为 %q，实际 %q", tc.in, tc.want, got)
		}
	}
}

func TestHintLine(t *testing.T) {
	shirt := goods.Item{Name: "一件 T恤", Price: 99}

	if got := hintLine(schedule.StatusWorking, shirt, 46); got != "一件 T恤 · 再过 46 分 就赚回来了" {
		t.Errorf("上班中提示不对: %q", got)
	}
	if got := hintLine(schedule.StatusNotStarted, shirt, 46); got != "一件 T恤 · 上班后再过 46 分 就赚回来了" {
		t.Errorf("未上班提示不对: %q", got)
	}
	if got := hintLine(schedule.StatusLunch, shirt, 46); got != "一件 T恤 · 午休中，赚钱速度暂停 ☕️" {
		t.Errorf("午休提示不对: %q", got)
	}
	if got := hintLine(schedule.StatusOffWork, shirt, 46); got != "一件 T恤 · 今天已经下班啦，明天继续 💪" {
		t.Errorf("下班提示不对: %q", got)
	}
	if got := hintLine(schedule.StatusWeekend, shirt, 46); got != "一件 T恤 · 今天休息，不赚钱也不花钱 🛌" {
		t.Errorf("周末提示不对: %q", got)
	}
}

func TestRefreshHintKeepsItemUntilAffordable(t *testing.T) {
	list := []goods.Item{
		{Name: "第一档", Price: 10},
		{Name: "第二档", Price: 100},
		{Name: "第三档", Price: 500},
		{Name: "第四档", Price: 700},
	}
	a := &App{goods: list, rnd: rand.New(rand.NewSource(1))}

	const daily = 1000

	a.refreshHint(0, daily)
	if a.hint == nil {
		t.Fatal("刚开始应该有可提示的物品")
	}
	picked := *a.hint

	// 还差 1 块钱，没买得起：不能换。
	a.refreshHint(picked.Price-1, daily)
	if a.hint == nil || *a.hint != picked {
		t.Fatalf("还没买得起时不该换物品，实际 %v", a.hint)
	}

	// 买得起了：换下一件，而且不能再是这一件。
	a.refreshHint(picked.Price, daily)
	if a.hint == nil {
		t.Fatal("买得起之后应该换下一件")
	}
	if *a.hint == picked {
		t.Error("已经买得起的东西不该再提示")
	}
	if a.hint.Price <= picked.Price {
		t.Errorf("下一件应该是还没买得起的更贵的东西，实际 %v", a.hint.Price)
	}
}

func TestRefreshHintSkipsItemsBeyondOneDay(t *testing.T) {
	list := []goods.Item{
		{Name: "一天买得起", Price: 100},
		{Name: "一天买不起", Price: 5000},
	}
	a := &App{goods: list, rnd: rand.New(rand.NewSource(1))}

	for i := 0; i < 20; i++ {
		a.hint = nil
		a.refreshHint(0, 1000)
		if a.hint == nil {
			t.Fatal("应该有可提示的物品")
		}
		if a.hint.Name == "一天买不起" {
			t.Fatal("一天赚不到的东西不该被提示")
		}
	}
}

func TestRefreshHintSkipsAffordableItems(t *testing.T) {
	list := []goods.Item{
		{Name: "买得起", Price: 10},
		{Name: "买不起", Price: 100},
	}
	a := &App{goods: list, rnd: rand.New(rand.NewSource(1))}

	for i := 0; i < 20; i++ {
		a.hint = nil
		a.refreshHint(50, 1000)
		if a.hint == nil {
			t.Fatal("应该有可提示的物品")
		}
		if a.hint.Name == "买得起" {
			t.Fatal("已经买得起的东西不该再提示")
		}
	}
}

func TestFillHintWithoutSalary(t *testing.T) {
	var s Snapshot
	fillHint(&s, schedule.StatusWorking, salary.Rates{}, 0, goods.Builtin, nil)

	if s.HintText != "设置年薪后，这里会告诉你还要多久能赚回来 🎯" {
		t.Errorf("未设年薪提示不对: %q", s.HintText)
	}
	if s.HintItem != "" {
		t.Errorf("未设年薪不该有物品: %q", s.HintItem)
	}
}

func TestFillHintWithoutAnyCandidate(t *testing.T) {
	rates := salary.FromAnnualSalary(240000, 420)

	var s Snapshot
	// 一天赚得到的东西都买得起了
	fillHint(&s, schedule.StatusWorking, rates, 100000, goods.Builtin, nil)
	if s.HintText != "今天赚得到的东西都买得起啦 🎉" {
		t.Errorf("都买得起提示不对: %q", s.HintText)
	}

	// 日薪太低，清单里没有一件是今天赚得到的
	var s2 Snapshot
	fillHint(&s2, schedule.StatusWorking, salary.FromAnnualSalary(100, 420), 0, goods.Builtin, nil)
	if s2.HintText != "按现在的年薪，一天赚不到清单里的东西，调高年薪试试 🎯" {
		t.Errorf("赚不到提示不对: %q", s2.HintText)
	}
}

func TestFillHintShowsProgress(t *testing.T) {
	rates := salary.FromAnnualSalary(240000, 420)
	item := &goods.Item{Name: "一件 T恤", Price: 99}

	var s Snapshot
	fillHint(&s, schedule.StatusWorking, rates, 49.5, goods.Builtin, item)

	if s.HintItem != "一件 T恤" {
		t.Errorf("物品不对: %q", s.HintItem)
	}
	if s.HintPrice != 99 {
		t.Errorf("价格不对: %v", s.HintPrice)
	}
	// 时薪 = 919.54/7 = 131.36，每分钟 2.19 元，还差 49.5 元 → 23 分钟
	if s.HintMinutes != 23 {
		t.Errorf("还差分钟数不对: %d", s.HintMinutes)
	}
	if s.HintProgress < 0.49 || s.HintProgress > 0.51 {
		t.Errorf("进度不对: %v", s.HintProgress)
	}
}

func TestUntilStartText(t *testing.T) {
	c := schedule.Default()
	now := time.Date(2026, 11, 3, 7, 30, 0, 0, time.Local)

	if got := untilStartText(now, c, schedule.StatusNotStarted); got != "距上班还有 1 小时" {
		t.Errorf("未上班提示不对: %q", got)
	}
	if got := untilStartText(now, c, schedule.StatusWorking); got != "" {
		t.Errorf("上班中不该显示距上班时间，实际 %q", got)
	}

	after := time.Date(2026, 11, 3, 18, 0, 0, 0, time.Local)
	got := untilStartText(after, c, schedule.StatusOffWork)
	// 18:00 到次日 8:30 是 14 小时 30 分
	if got != "距明天上班还有 14 小时 30 分" {
		t.Errorf("下班后提示不对: %q", got)
	}

	// 午休时指向下午上班
	lunch := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	if got := untilStartText(lunch, c, schedule.StatusLunch); got != "距下午上班还有 1 小时" {
		t.Errorf("午休提示不对: %q", got)
	}
}

func TestUntilStartTextSkipsWeekend(t *testing.T) {
	c := schedule.Default()

	// 周五 18:00 已下班：下一个工作日是周一 8:30，不是"明天"周六
	fri := time.Date(2026, 11, 6, 18, 0, 0, 0, time.Local)
	if got := untilStartText(fri, c, schedule.StatusOffWork); got != "距星期一上班还有 62 小时 30 分" {
		t.Errorf("周五晚上提示不对: %q", got)
	}

	// 周六上午：休息日，同样指向周一 8:30
	sat := time.Date(2026, 11, 7, 10, 0, 0, 0, time.Local)
	if got := untilStartText(sat, c, schedule.StatusWeekend); got != "距星期一上班还有 46 小时 30 分" {
		t.Errorf("周六提示不对: %q", got)
	}
}

func TestTimerSignature(t *testing.T) {
	base := settings.Default()
	if timerSignature(base) != timerSignature(settings.Default()) {
		t.Error("同样的配置指纹应一致")
	}

	changed := settings.Default()
	changed.AnnualSalary = 100000
	if timerSignature(base) == timerSignature(changed) {
		t.Error("年薪变了指纹应不同")
	}

	moved := settings.Default()
	moved.PetX, moved.PetY = 999, 888
	if timerSignature(base) != timerSignature(moved) {
		t.Error("宠物位置不影响计时，指纹不应变化")
	}
}

func TestToneOf(t *testing.T) {
	cases := map[schedule.Status]string{
		schedule.StatusWorking:    "working",
		schedule.StatusLunch:      "lunch",
		schedule.StatusOffWork:    "off",
		schedule.StatusWeekend:    "rest",
		schedule.StatusNotStarted: "idle",
	}
	for st, want := range cases {
		if got := toneOf(st); got != want {
			t.Errorf("toneOf(%s) 应为 %s，实际 %s", st, want, got)
		}
	}
}

func TestWeekdayCN(t *testing.T) {
	// 2026-11-03 是星期二
	d := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	if got := weekdayCN(d); got != "星期二" {
		t.Errorf("星期应为 星期二，实际 %s", got)
	}
}

func TestDaySeed(t *testing.T) {
	d := time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local)
	if got := daySeed(d); got != 20261103 {
		t.Errorf("daySeed 应为 20261103，实际 %d", got)
	}
}
