package settings

import (
	"path/filepath"
	"testing"

	"timer/internal/goods"
)

func TestDefaultSettings(t *testing.T) {
	d := Default()
	if d.WorkStart != "08:30" || d.LunchStart != "11:30" || d.WorkResume != "13:00" || d.WorkEnd != "17:00" {
		t.Errorf("默认时间不对: %+v", d)
	}
	if !d.AlwaysOnTop {
		t.Error("默认应置顶")
	}
	if d.Theme != ThemeDark {
		t.Errorf("默认应为深色主题，实际 %s", d.Theme)
	}
}

func TestNormalizeFixesBadTheme(t *testing.T) {
	if got := (Settings{Theme: "粉红"}).Normalize().Theme; got != ThemeDark {
		t.Errorf("非法主题应回退深色，实际 %s", got)
	}
	if got := (Settings{Theme: ThemeLight}).Normalize().Theme; got != ThemeLight {
		t.Errorf("浅色主题应保留，实际 %s", got)
	}
	if got := (Settings{}).Normalize().Theme; got != ThemeDark {
		t.Errorf("空主题应回退深色，实际 %s", got)
	}
}

func TestIsLight(t *testing.T) {
	if !(Settings{Theme: ThemeLight}).IsLight() {
		t.Error("浅色主题 IsLight 应为 true")
	}
	if (Settings{Theme: ThemeDark}).IsLight() {
		t.Error("深色主题 IsLight 应为 false")
	}
}

func TestNormalizeFixesBadValues(t *testing.T) {
	s := Settings{AnnualSalary: -100, WorkStart: "乱写", LunchStart: "25:00"}
	s = s.Normalize()
	if s.AnnualSalary != 0 {
		t.Errorf("负年薪应归零，实际 %v", s.AnnualSalary)
	}
	if s.WorkStart != "08:30" {
		t.Errorf("非法上班时间应回退默认，实际 %s", s.WorkStart)
	}
	if s.LunchStart != "11:30" {
		t.Errorf("非法午休时间应回退默认，实际 %s", s.LunchStart)
	}
}

func TestScheduleConversion(t *testing.T) {
	c := Settings{
		WorkStart: "09:00", LunchStart: "12:00",
		WorkResume: "13:30", WorkEnd: "18:00",
	}.Schedule()
	if c.WorkStart != 540 || c.LunchStart != 720 || c.WorkResume != 810 || c.WorkEnd != 1080 {
		t.Errorf("排班转换错误: %+v", c)
	}
	// 09:00–12:00 是 180 分钟，13:30–18:00 是 270 分钟，午休不计
	if got := c.TotalWorkMinutes(); got != 450 {
		t.Errorf("总工时应为 450 分钟，实际 %d", got)
	}
}

func TestAllGoods(t *testing.T) {
	s := Default()
	s.CustomGoods = append(s.CustomGoods, goods.Item{Name: "一杯豆浆", Price: 8})
	all := s.AllGoods()
	if len(all) != len(goods.Builtin)+1 {
		t.Errorf("物品总数应为 内置+1，实际 %d", len(all))
	}
	if all[len(all)-1].Name != "一杯豆浆" {
		t.Errorf("自定义物品应排在最后，实际 %s", all[len(all)-1].Name)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	s := Default()
	s.AnnualSalary = 360000
	s.WorkEnd = "18:30"
	if err := SaveTo(path, s); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.AnnualSalary != 360000 || got.WorkEnd != "18:30" {
		t.Errorf("读回来的配置不对: %+v", got)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	got, err := LoadFrom(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("文件不存在不应报错: %v", err)
	}
	if got.AnnualSalary != Default().AnnualSalary {
		t.Errorf("文件不存在应返回默认配置，实际 %+v", got)
	}
}
