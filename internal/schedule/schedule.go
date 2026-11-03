// Package schedule 描述一天的工作安排，并据此推算当前状态与工时。
//
// 一天被切成四段：未上班、上午上班、午休、下午上班、已下班。
// 真正"工作"的时间只有上午与下午两段，午休不计入工时。
package schedule

import (
	"fmt"
	"time"
)

// Status 表示一天中某个时刻所处的阶段。
type Status string

const (
	// StatusNotStarted 还没到上班时间。
	StatusNotStarted Status = "未上班"
	// StatusWorking 正在上班（含上午与下午两段）。
	StatusWorking Status = "上班中"
	// StatusLunch 处于午休。
	StatusLunch Status = "午休中"
	// StatusOffWork 已经下班。
	StatusOffWork Status = "已下班"
)

// Segment 是以"当天 0 点起的分钟数"表示的半开区间 [Start, End)。
type Segment struct {
	Start int
	End   int
}

// Config 描述一天的工作安排，四个字段均为当天 0 点起的分钟数。
type Config struct {
	WorkStart  int // 上班，如 8:30 → 510
	LunchStart int // 午休开始，如 11:30 → 690
	WorkResume int // 下午上班，如 13:00 → 780
	WorkEnd    int // 下班，如 17:00 → 1020
}

// Default 返回规格里给出的默认安排：8:30 上班，11:30 午休，13:00 下午上班，17:00 下班。
func Default() Config {
	return Config{WorkStart: 510, LunchStart: 690, WorkResume: 780, WorkEnd: 1020}
}

// WorkSegments 返回实际工作的两个区间，午休不在其中。
func (c Config) WorkSegments() []Segment {
	return []Segment{
		{Start: c.WorkStart, End: c.LunchStart},
		{Start: c.WorkResume, End: c.WorkEnd},
	}
}

// TotalWorkMinutes 返回一天实际工作分钟数（不含午休）。
func (c Config) TotalWorkMinutes() int {
	total := 0
	for _, s := range c.WorkSegments() {
		total += s.End - s.Start
	}
	return total
}

// StatusAt 返回 t 时刻所处的阶段。
func (c Config) StatusAt(t time.Time) Status {
	switch m := MinutesOfDay(t); {
	case m < c.WorkStart:
		return StatusNotStarted
	case m < c.LunchStart:
		return StatusWorking
	case m < c.WorkResume:
		return StatusLunch
	case m < c.WorkEnd:
		return StatusWorking
	default:
		return StatusOffWork
	}
}

// WorkedMinutes 返回 t 时刻当天已经工作了多少分钟。已下班时返回全天工时。
func (c Config) WorkedMinutes(t time.Time) int {
	m := MinutesOfDay(t)
	worked := 0
	for _, s := range c.WorkSegments() {
		worked += overlap(s, m)
	}
	return worked
}

// Until 返回从 t 到当天 minuteOfDay 那一刻还有多久，精确到秒；已经过了则返回 0。
func Until(t time.Time, minuteOfDay int) time.Duration {
	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	d := midnight.Add(time.Duration(minuteOfDay) * time.Minute).Sub(t)
	if d < 0 {
		return 0
	}
	return d
}

// MinutesOfDay 返回 t 当天已经过去的分钟数（忽略秒）。
func MinutesOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

// overlap 返回区间 s 与"从 0 到 m"的重叠长度。
func overlap(s Segment, m int) int {
	if m <= s.Start {
		return 0
	}
	if m >= s.End {
		return s.End - s.Start
	}
	return m - s.Start
}

// ParseHM 把 "HH:MM" 解析成当天 0 点起的分钟数；格式非法时返回 false。
func ParseHM(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// FormatHM 把分钟数格式化成 "HH:MM"。
func FormatHM(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
