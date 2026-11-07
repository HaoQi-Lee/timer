// fishicon 把 internal/fishicon 的像素大肥鱼渲染成 PNG 预览：
//
//	go run ./tools/fishicon out.png
//
// 每格放大成 20×20 的块，垫一张深浅棋盘格底（看清透明区域）。
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"timer/internal/fishicon"
)

func main() {
	out := "fishicon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	const cell = 20
	gw, gh := len(fishicon.Pixels[0]), len(fishicon.Pixels)
	img := image.NewRGBA(image.Rect(0, 0, gw*cell, gh*cell))

	// 棋盘格底：透明区域看起来是洞，不是黑色
	for y := 0; y < gh*cell; y++ {
		for x := 0; x < gw*cell; x++ {
			c := color.RGBA{70, 74, 90, 255}
			if (x/cell+y/cell)%2 == 0 {
				c = color.RGBA{46, 49, 62, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}

	px := fishicon.Render(gw*cell, gh*cell)
	for i := 0; i < gw*cell*gh*cell; i++ {
		if px[i*4+3] == 0 {
			continue
		}
		img.SetRGBA(i%(gw*cell), i/(gw*cell), color.RGBA{
			R: px[i*4], G: px[i*4+1], B: px[i*4+2], A: px[i*4+3],
		})
	}

	f, err := os.Create(out)
	if err != nil {
		fmt.Println("写文件失败:", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Println("编码 PNG 失败:", err)
		os.Exit(1)
	}
	fmt.Println("已生成", out)
}
