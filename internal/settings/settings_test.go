package settings

import (
	"os"
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

func TestNormalizePetAlwaysOnTop(t *testing.T) {
	// 没选过 → 视为置顶
	var empty Settings
	if !empty.PetOnTop() {
		t.Error("没选过置顶时 PetOnTop 应为 true")
	}
	empty = empty.Normalize()
	if empty.PetAlwaysOnTop == nil || !*empty.PetAlwaysOnTop {
		t.Errorf("Normalize 应把 nil 补成 true，实际 %v", empty.PetAlwaysOnTop)
	}
	// 明确选了不置顶 → 保持 false
	no := false
	s := Settings{PetAlwaysOnTop: &no}.Normalize()
	if s.PetOnTop() {
		t.Error("明确选择不置顶后 PetOnTop 应为 false")
	}
}

func TestSaveAtomicLeavesNoTmp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	// 覆盖保存后不应留下临时文件
	if err := SaveTo(path, Default()); err != nil {
		t.Fatalf("二次保存失败: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("不应残留临时文件: %v", err)
	}
	if _, err := LoadFrom(path); err != nil {
		t.Fatalf("原子替换后应能正常读取: %v", err)
	}
}

func TestNewFieldsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	s.AutoStart = true
	mute := false
	s.NotifyOffWork = &mute
	if err := SaveTo(path, s); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !got.AutoStart {
		t.Error("开机自启应能往返保存")
	}
	if got.WantOffWorkNotify() {
		t.Error("下班提醒关掉后应保持关闭")
	}
}

func TestOldConfigUpgradesKeepDefaults(t *testing.T) {
	// 老版本配置文件里没有新字段，升级后应按默认处理：置顶、弹下班提醒
	path := filepath.Join(t.TempDir(), "settings.json")
	old := `{"annualSalary":240000,"workStart":"08:30","lunchStart":"11:30","workResume":"13:00","workEnd":"17:00"}`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !got.PetOnTop() {
		t.Error("老配置升级后宠物应默认置顶")
	}
	if !got.WantOffWorkNotify() {
		t.Error("老配置升级后应默认弹下班提醒")
	}
}

// TestLoadFromBOM 带 UTF-8 BOM 的配置要能正常解析（记事本/PowerShell 写出的样子）。
func TestLoadFromBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	bom := []byte{0xEF, 0xBB, 0xBF}
	if err := os.WriteFile(path, append(bom, []byte(`{"annualSalary":123456}`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("带 BOM 解析失败: %v", err)
	}
	if s.AnnualSalary != 123456 {
		t.Fatalf("AnnualSalary = %v，要 123456", s.AnnualSalary)
	}
}
