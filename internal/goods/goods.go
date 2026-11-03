// Package goods 内置用于"赚回来"估算的常见物品与价格。
package goods

import "math/rand"

// Item 是一件物品及其价格，Name 里带上量词方便直接拼成一句话。
type Item struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Emoji string  `json:"emoji"`
}

// Builtin 是内置的物品清单，覆盖从十几块的小零食到几千块的电子产品。
//
// 名称都按「X 就赚回来了」这句话读起来顺当来取名，比如"理一次发"而不是"一次理发"。
var Builtin = []Item{
	{Name: "一杯奶茶", Price: 15, Emoji: "🧋"},
	{Name: "一个面包", Price: 12, Emoji: "🥐"},
	{Name: "一杯咖啡", Price: 18, Emoji: "☕"},
	{Name: "打车一次", Price: 25, Emoji: "🚕"},
	{Name: "点一份外卖", Price: 32, Emoji: "🍱"},
	{Name: "看场电影", Price: 45, Emoji: "🎬"},
	{Name: "理一次发", Price: 50, Emoji: "💈"},
	{Name: "一件 T恤", Price: 99, Emoji: "👕"},
	{Name: "一双运动鞋", Price: 399, Emoji: "👟"},
	{Name: "一支口红", Price: 320, Emoji: "💄"},
	{Name: "做一次按摩", Price: 298, Emoji: "💆"},
	{Name: "一台显示器", Price: 1299, Emoji: "🖥️"},
	{Name: "一台笔记本电脑", Price: 5999, Emoji: "💻"},
	{Name: "一场旅行", Price: 3000, Emoji: "✈️"},
}

// Shuffled 返回按 seed 洗牌后的副本。同一个 seed 总是得到同样的顺序，
// 这样"今天"看到的轮播顺序是稳定的，跨天才会变。
func Shuffled(items []Item, seed int64) []Item {
	out := make([]Item, len(items))
	copy(out, items)
	rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}
