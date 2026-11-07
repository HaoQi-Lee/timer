package fishicon

import "testing"

// TestPixelsValid 网格本身要规整：16 行 × 16 列，字符都在调色板里。
func TestPixelsValid(t *testing.T) {
	if len(Pixels) != 16 {
		t.Fatalf("行数 = %d，要 16", len(Pixels))
	}
	for y, row := range Pixels {
		if len(row) != 16 {
			t.Errorf("第 %d 行长度 = %d，要 16: %q", y, len(row), row)
		}
		for x := 0; x < len(row); x++ {
			if _, ok := Palette[row[x]]; !ok {
				t.Errorf("第 %d 行第 %d 列字符 %q 不在调色板里", y, x, row[x])
			}
		}
	}
}

// TestRenderSize 渲染尺寸要对，且至少有一个不透明像素。
func TestRenderSize(t *testing.T) {
	for _, sz := range [][2]int{{16, 16}, {20, 20}, {24, 24}, {32, 32}} {
		px := Render(sz[0], sz[1])
		if len(px) != sz[0]*sz[1]*4 {
			t.Fatalf("Render(%d,%d) 长度 = %d", sz[0], sz[1], len(px))
		}
		opaque := false
		for i := 0; i < sz[0]*sz[1]; i++ {
			if px[i*4+3] != 0 {
				opaque = true
				break
			}
		}
		if !opaque {
			t.Errorf("Render(%d,%d) 全透明", sz[0], sz[1])
		}
	}
}
