// Package salary 由年薪折算出日薪、时薪、每分钟收入，并推算已赚金额与"还要多久赚回来"。
package salary

import "math"

// WorkdaysPerYear 每年法定工作日：52 周 × 5 天 − 11 天法定节假日 = 261 天。
const WorkdaysPerYear = 261

// Rates 是一组由年薪推导出来的金额与速度。
type Rates struct {
	// DailyRate 每日工资。
	DailyRate float64 `json:"dailyRate"`
	// HourlyRate 每小时工资。
	HourlyRate float64 `json:"hourlyRate"`
	// MinuteRate 每分钟工资。
	MinuteRate float64 `json:"minuteRate"`
}

// FromAnnualSalary 由年薪与每天实际工作分钟数折算出各项速度。
//
// 每天实际工作分钟数只影响时薪与分钟薪，日薪只由年薪与法定工作日决定。
func FromAnnualSalary(annual float64, workMinutesPerDay int) Rates {
	daily := annual / WorkdaysPerYear
	if workMinutesPerDay <= 0 {
		return Rates{DailyRate: daily}
	}
	return Rates{
		DailyRate:  daily,
		HourlyRate: daily * 60 / float64(workMinutesPerDay),
		MinuteRate: daily / float64(workMinutesPerDay),
	}
}

// Earned 返回 workedMinutes 分钟内已经赚到的钱。
func (r Rates) Earned(workedMinutes int) float64 {
	if r.MinuteRate <= 0 || workedMinutes <= 0 {
		return 0
	}
	return r.MinuteRate * float64(workedMinutes)
}

// MinutesToEarn 返回按当前速度还要工作多少分钟才能赚到 price 元；不够一整分钟则向上取整。
func (r Rates) MinutesToEarn(price float64) int {
	if r.MinuteRate <= 0 {
		return 0
	}
	return int(math.Ceil(price / r.MinuteRate))
}

// MinutesToEarnFrom 返回从已赚 earned 元起算，还要工作多少分钟才能赚到 price 元。
//
// 和 MinutesToEarn 的区别是它扣掉了已经赚到的部分：提示区要的是"还要多久"，
// 不是"从零开始要多久"。
func (r Rates) MinutesToEarnFrom(earned, price float64) int {
	if r.MinuteRate <= 0 {
		return 0
	}
	remaining := price - earned
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining / r.MinuteRate))
}
