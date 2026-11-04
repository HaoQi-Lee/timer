//go:build ignore

// 一次性工具：统计 PNG 里的颜色分布，判断"透明区域"到底是不是纯黑。
package main

import (
	"fmt"
	"image/png"
	"os"
	"sort"
)

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
	counts := map[string]int{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			key := fmt.Sprintf("#%02x%02x%02x a=%02x", r>>8, g>>8, bl>>8, a>>8)
			counts[key]++
			_ = a
		}
	}
	type kv struct {
		k string
		n int
	}
	var list []kv
	for k, n := range counts {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	total := b.Dx() * b.Dy()
	fmt.Printf("%s %dx%d 共 %d 种颜色，前 8 名：\n", os.Args[1], b.Dx(), b.Dy(), len(counts))
	for i, e := range list {
		if i >= 8 {
			break
		}
		fmt.Printf("  %s  %d px (%.1f%%)\n", e.k, e.n, float64(e.n)*100/float64(total))
	}
}
