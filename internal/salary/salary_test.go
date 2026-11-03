package salary

import (
	"math"
	"testing"
)

const epsilon = 1e-9

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestFromAnnualSalary(t *testing.T) {
	r := FromAnnualSalary(240000, 420)
	if !approx(r.DailyRate, 240000.0/261) {
		t.Errorf("日薪应为 %v，实际 %v", 240000.0/261, r.DailyRate)
	}
	if !approx(r.HourlyRate, r.DailyRate*60/420) {
		t.Errorf("时薪不对: %v", r.HourlyRate)
	}
	if !approx(r.MinuteRate, r.DailyRate/420) {
		t.Errorf("分钟薪不对: %v", r.MinuteRate)
	}
}

func TestWorkdaysPerYear(t *testing.T) {
	if WorkdaysPerYear != 261 {
		t.Errorf("法定工作日应为 261，实际 %d", WorkdaysPerYear)
	}
}

func TestEarned(t *testing.T) {
	r := FromAnnualSalary(240000, 420)
	// 上午 3 小时（180 分钟）≈ 日薪的 3/7
	want := r.DailyRate * 180.0 / 420.0
	if got := r.Earned(180); !approx(got, want) {
		t.Errorf("已赚应为 %v，实际 %v", want, got)
	}
	if got := r.Earned(0); got != 0 {
		t.Errorf("未上班时已赚应为 0，实际 %v", got)
	}
	// 全天工时正好等于日薪
	if got := r.Earned(420); !approx(got, r.DailyRate) {
		t.Errorf("全天已赚应等于日薪 %v，实际 %v", r.DailyRate, got)
	}
}

func TestMinutesToEarn(t *testing.T) {
	r := FromAnnualSalary(240000, 420)
	// 一件 99 元的 T恤：99 / (919.54/420) ≈ 45.2 分钟，向上取整 46
	if got := r.MinutesToEarn(99); got != 46 {
		t.Errorf("赚回 99 元应需 46 分钟，实际 %d", got)
	}
	if got := r.MinutesToEarn(0); got != 0 {
		t.Errorf("价格 0 应得 0 分钟，实际 %d", got)
	}
}

func TestZeroAnnualSalary(t *testing.T) {
	r := FromAnnualSalary(0, 420)
	if r.DailyRate != 0 || r.HourlyRate != 0 || r.MinuteRate != 0 {
		t.Errorf("年薪为 0 时各速率都应为 0，实际 %+v", r)
	}
	if got := r.Earned(100); got != 0 {
		t.Errorf("年薪为 0 时已赚应为 0，实际 %v", got)
	}
}
