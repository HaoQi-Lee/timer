package schedule

import (
	"testing"
	"time"
)

func at(t *testing.T, hhmm string) time.Time {
	t.Helper()
	h, m := 0, 0
	for i := 0; i < len(hhmm); i++ {
		if hhmm[i] == ':' {
			h = m
			m = 0
			continue
		}
		m = m*10 + int(hhmm[i]-'0')
	}
	return time.Date(2026, 11, 3, h, m, 0, 0, time.Local)
}

func TestDefaultSchedule(t *testing.T) {
	c := Default()
	if c.WorkStart != 8*60+30 || c.LunchStart != 11*60+30 || c.WorkResume != 13*60 || c.WorkEnd != 17*60 {
		t.Fatalf("默认排班不对: %+v", c)
	}
	// 3 小时上午 + 4 小时下午 = 7 小时，午休不计入
	if got := c.TotalWorkMinutes(); got != 420 {
		t.Fatalf("总工时应为 420 分钟，实际 %d", got)
	}
}

func TestStatusAt(t *testing.T) {
	c := Default()
	cases := []struct {
		at   string
		want Status
	}{
		{"07:59", StatusNotStarted},
		{"08:00", StatusNotStarted},
		{"08:30", StatusWorking},
		{"11:29", StatusWorking},
		{"11:30", StatusLunch},
		{"12:59", StatusLunch},
		{"13:00", StatusWorking},
		{"16:59", StatusWorking},
		{"17:00", StatusOffWork},
		{"23:59", StatusOffWork},
	}
	for _, tc := range cases {
		if got := c.StatusAt(at(t, tc.at)); got != tc.want {
			t.Errorf("%s 状态应为 %s，实际 %s", tc.at, tc.want, got)
		}
	}
}

func TestWorkedMinutes(t *testing.T) {
	c := Default()
	cases := []struct {
		at   string
		want int
	}{
		{"00:00", 0},
		{"08:29", 0},
		{"08:30", 0},
		{"09:00", 30},
		{"11:30", 180}, // 上午 3 小时结束
		{"12:00", 180}, // 午休不涨
		{"13:00", 180}, // 下午还没开始
		{"13:30", 210},
		{"17:00", 420}, // 全天工时
		{"23:00", 420},
	}
	for _, tc := range cases {
		if got := c.WorkedMinutes(at(t, tc.at)); got != tc.want {
			t.Errorf("%s 已工作应为 %d 分钟，实际 %d", tc.at, tc.want, got)
		}
	}
}

func TestUntil(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 30, 0, 0, time.Local)
	if got := Until(now, 17*60); got != 90*time.Minute {
		t.Errorf("距下班应为 90 分钟，实际 %v", got)
	}
	// 已经过了下班时间，返回 0
	if got := Until(now, 9*60); got != 0 {
		t.Errorf("已过时间应返回 0，实际 %v", got)
	}
}

func TestParseAndFormatHM(t *testing.T) {
	if v, ok := ParseHM("08:30"); !ok || v != 510 {
		t.Errorf("解析 08:30 应得 510，实际 %d %v", v, ok)
	}
	if _, ok := ParseHM("25:00"); ok {
		t.Error("25:00 应解析失败")
	}
	if _, ok := ParseHM("abc"); ok {
		t.Error("abc 应解析失败")
	}
	if got := FormatHM(510); got != "08:30" {
		t.Errorf("格式化应为 08:30，实际 %s", got)
	}
}
