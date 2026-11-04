//go:build ignore

// 一次性工具：把桌宠素材的不透明轮廓（含 hitMargin 外扩）画成 ASCII，
// 用来挑"真空白"的坐标验证点击穿透。
package main

import (
	"fmt"
	"image/png"
	"os"
)

const hitMargin = 0

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Println("打开失败:", err)
		os.Exit(1)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Println("解码失败:", err)
		os.Exit(1)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	opaque := make([]bool, w*h)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 128 {
				opaque[(y-b.Min.Y)*w+(x-b.Min.X)] = true
			}
		}
	}
	margin := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !opaque[y*w+x] {
				continue
			}
			for d := -hitMargin; d <= hitMargin; d++ {
				if xx := x + d; xx >= 0 && xx < w {
					margin[y*w+xx] = true
				}
			}
		}
	}
	dil := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !margin[y*w+x] {
				continue
			}
			for d := -hitMargin; d <= hitMargin; d++ {
				if yy := y + d; yy >= 0 && yy < h {
					dil[yy*w+x] = true
				}
			}
		}
	}

	// 逐行原样输出太宽，横向 1/2 采样：两列并一列，任一命中就算命中
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x += 2 {
			c := '·'
			if dil[y*w+x] || (y+1 < h && dil[(y+1)*w+x]) ||
				(x+1 < w && dil[y*w+x+1]) || (x+1 < w && y+1 < h && dil[(y+1)*w+x+1]) {
				c = '█'
			}
			fmt.Printf("%c", c)
		}
		fmt.Println()
	}
	fmt.Printf("尺寸 %dx%d，每字符 = 2x2 像素\n", w, h)
}
