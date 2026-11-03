package main

import (
	"testing"
	"time"

	"timer/internal/goods"
	"timer/internal/schedule"
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

func TestHintText(t *testing.T) {
	shirt := goods.Item{Name: "一件 T恤", Price: 99}

	if got := hintText(schedule.StatusWorking, shirt, 0, 46, 2.19); got != "再过 46 分，一件 T恤 就赚回来了" {
		t.Errorf("上班中提示不对: %q", got)
	}
	if got := hintText(schedule.StatusNotStarted, shirt, 0, 46, 2.19); got != "上班后再过 46 分，一件 T恤 就赚回来了" {
		t.Errorf("未上班提示不对: %q", got)
	}
	if got := hintText(schedule.StatusWorking, shirt, 500, 0, 2.19); got != "一件 T恤 已经赚回来了 🎉" {
		t.Errorf("已赚够提示不对: %q", got)
	}
	if got := hintText(schedule.StatusLunch, shirt, 0, 46, 2.19); got != "午休中，赚钱速度暂停，稍后再算 ☕️" {
		t.Errorf("午休提示不对: %q", got)
	}
	if got := hintText(schedule.StatusOffWork, shirt, 0, 46, 2.19); got != "今天已经下班啦，明天继续 💪" {
		t.Errorf("下班提示不对: %q", got)
	}
	if got := hintText(schedule.StatusWorking, shirt, 0, 0, 0); got != "设置年薪后，这里会告诉你还要多久能赚回来 🎯" {
		t.Errorf("未设年薪提示不对: %q", got)
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
}

func TestToneOf(t *testing.T) {
	cases := map[schedule.Status]string{
		schedule.StatusWorking:    "working",
		schedule.StatusLunch:      "lunch",
		schedule.StatusOffWork:    "off",
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
