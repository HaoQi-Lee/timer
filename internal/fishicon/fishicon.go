// Package fishicon 用手绘像素网格生成"大肥鱼"（桌宠那只鲸鱼娘）的图标像素。
//
// 形象对着 dsh-pet 的角色参考图（tools/fishicon 可渲染 PNG 预览核对）：
// 二头身 Q 版——深蓝紫大圆头、白色蕾丝女仆头巾、头顶呆毛、大蓝眼、
// 腮红、藏青小身体白围裙、右侧月牙尾鳍。
package fishicon

// Palette 是网格字符到颜色的映射（R, G, B, A，均为 0-255）。
var Palette = map[byte][4]byte{
	'K': {43, 53, 96, 255},   // 描边 & 藏青裙
	'B': {74, 90, 168, 255},  // 头发/身体主色（蓝紫）
	'L': {127, 196, 236, 255},// 发梢亮蓝 & 尾鳍
	'E': {46, 95, 191, 255},  // 眼睛
	'W': {245, 245, 245, 255},// 白（头巾 / 围裙 / 眼睛高光）
	'R': {240, 184, 168, 255},// 腮红
	'.': {0, 0, 0, 0},        // 透明
}

// Pixels 是 16×16 的像素画。
//
// 行 0-2 呆毛（2px 宽弯钩，从蕾丝里钻出来）；行 2-3 白色蕾丝头巾
// （下缘锯齿：齿 W 齿 W 齿）；行 4-9 大圆头；行 6-8 三像素宽的大蓝眼
// （高光统一在左上）+ 腮红；行 9 发梢渐变亮蓝；行 10-13 藏青小身体
// + 白围裙 + 白色裙摆荷叶边；行 10-12 右侧月牙尾鳍。
var Pixels = []string{
	"......BB........",
	".......BB.......",
	"....KWWBBWWK....",
	"...KWWBWWBWWK...",
	"..KBBBBBBBBBBK..",
	"..KBBBBBBBBBBK..",
	"..KBWEEBBWEEBK..",
	"..KBEEEBBEEEBK..",
	"..KREEEBBEEERK..",
	"...KLLLLLLLLK...",
	".....KKKKKK...L.",
	"....KKWWWWKK.LL.",
	"...KKWWWWWWKKL..",
	".....KK..KK.....",
	"................",
	"................",
}

// Render 把网格最近邻放大到 w×h，返回 RGBA 像素（w*h*4 字节）。
func Render(w, h int) []byte {
	gw, gh := len(Pixels[0]), len(Pixels)
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		row := Pixels[y*gh/h]
		for x := 0; x < w; x++ {
			c := Palette[row[x*gw/w]]
			copy(out[(y*w+x)*4:], c[:])
		}
	}
	return out
}
