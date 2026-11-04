// Package settings 负责配置的默认值、持久化与合法性修正。
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"

	"timer/internal/goods"
	"timer/internal/schedule"
)

// 主题取值。
const (
	// ThemeDark 深色主题。
	ThemeDark = "dark"
	// ThemeLight 浅色主题。
	ThemeLight = "light"
)

// 桌宠表情包取值。Go 侧只认这几个键，名字和台词都在前端。
const (
	// PetPackClassic 咸鱼（默认）。
	PetPackClassic = "classic"
	// PetPackHappy 元气。
	PetPackHappy = "happy"
	// PetPackFoodie 干饭。
	PetPackFoodie = "foodie"
	// PetPackSleepy 睡觉。
	PetPackSleepy = "sleepy"
	// PetPackAngry 暴躁。
	PetPackAngry = "angry"
)

// PetPacks 是全部合法的表情包键。
var PetPacks = []string{PetPackClassic, PetPackHappy, PetPackFoodie, PetPackSleepy, PetPackAngry}

// ValidPetPack 判断是不是合法的表情包键。
func ValidPetPack(pack string) bool {
	for _, p := range PetPacks {
		if p == pack {
			return true
		}
	}
	return false
}

// Settings 是程序的全部可配置项。
type Settings struct {
	// AnnualSalary 年薪（元），用于折算日薪。
	AnnualSalary float64 `json:"annualSalary"`
	// WorkStart 上班时间，"HH:MM"。
	WorkStart string `json:"workStart"`
	// LunchStart 午休开始时间，"HH:MM"。
	LunchStart string `json:"lunchStart"`
	// WorkResume 下午上班时间，"HH:MM"。
	WorkResume string `json:"workResume"`
	// WorkEnd 下班时间，"HH:MM"。
	WorkEnd string `json:"workEnd"`
	// AlwaysOnTop 窗口是否置顶。
	AlwaysOnTop bool `json:"alwaysOnTop"`
	// Theme 界面主题，"dark" 或 "light"。
	Theme string `json:"theme"`
	// CustomGoods 用户自己添加的物品，会追加在内置清单之后。
	CustomGoods []goods.Item `json:"customGoods"`
	// WindowX / WindowY 记住窗口位置，负数表示还没记住过。
	WindowX int `json:"windowX"`
	WindowY int `json:"windowY"`
	// PetEnabled 要不要开启桌面宠物（大肥鱼）。
	PetEnabled bool `json:"petEnabled"`
	// PetPack 宠物当前的表情包。
	PetPack string `json:"petPack"`
	// PetX / PetY 宠物窗口位置，0 表示还没记住过。
	PetX int `json:"petX"`
	PetY int `json:"petY"`
}

// Default 返回默认配置：年薪 24 万，8:30–17:00，午休 11:30–13:00，窗口置顶，深色主题。
func Default() Settings {
	return Settings{
		AnnualSalary: 240000,
		WorkStart:    "08:30",
		LunchStart:   "11:30",
		WorkResume:   "13:00",
		WorkEnd:      "17:00",
		AlwaysOnTop:  true,
		Theme:        ThemeDark,
		CustomGoods:  nil,
	}
}

// Normalize 修正不合法的字段：时间格式错误时回退到默认值，年薪为负时归零，
// 主题不是 dark/light 时回退到深色。
func (s Settings) Normalize() Settings {
	d := Default()
	if _, ok := schedule.ParseHM(s.WorkStart); !ok {
		s.WorkStart = d.WorkStart
	}
	if _, ok := schedule.ParseHM(s.LunchStart); !ok {
		s.LunchStart = d.LunchStart
	}
	if _, ok := schedule.ParseHM(s.WorkResume); !ok {
		s.WorkResume = d.WorkResume
	}
	if _, ok := schedule.ParseHM(s.WorkEnd); !ok {
		s.WorkEnd = d.WorkEnd
	}
	if s.AnnualSalary < 0 {
		s.AnnualSalary = 0
	}
	if s.Theme != ThemeDark && s.Theme != ThemeLight {
		s.Theme = d.Theme
	}
	if !ValidPetPack(s.PetPack) {
		s.PetPack = PetPackClassic
	}
	return s
}

// IsLight 判断是不是浅色主题。
func (s Settings) IsLight() bool {
	return s.Theme == ThemeLight
}

// Schedule 把配置里的时间字符串转成排班表。
func (s Settings) Schedule() schedule.Config {
	c := schedule.Default()
	c.WorkStart, _ = schedule.ParseHM(s.WorkStart)
	c.LunchStart, _ = schedule.ParseHM(s.LunchStart)
	c.WorkResume, _ = schedule.ParseHM(s.WorkResume)
	c.WorkEnd, _ = schedule.ParseHM(s.WorkEnd)
	return c
}

// AllGoods 返回内置清单加上用户自定义清单。
func (s Settings) AllGoods() []goods.Item {
	all := make([]goods.Item, 0, len(goods.Builtin)+len(s.CustomGoods))
	all = append(all, goods.Builtin...)
	all = append(all, s.CustomGoods...)
	return all
}

// Path 返回配置文件的完整路径。
//
// 可以用 WORK_TIMER_CONFIG 环境变量指定别的路径，方便做便携版或临时配置。
func Path() (string, error) {
	if custom := os.Getenv("WORK_TIMER_CONFIG"); custom != "" {
		return custom, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "work-timer", "settings.json"), nil
}

// Load 读取默认位置上的配置。文件不存在时返回默认配置；解析失败时返回默认配置与错误。
func Load() (Settings, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	return LoadFrom(path)
}

// LoadFrom 从指定路径读取配置。
func LoadFrom(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Default(), err
	}
	return s.Normalize(), nil
}

// Save 把配置写入默认位置。
func Save(s Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	return SaveTo(path, s)
}

// SaveTo 把配置写入指定路径。
func SaveTo(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.Normalize(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
