// Package goods 内置用于"赚回来"估算的常见物品与价格。
package goods

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

// Reachable 返回今天一天赚得到的物品，也就是价格不超过日薪的那些。
//
// 一天能赚到的钱有上限（就是日薪），所以"一场旅行"这种几千块的东西
// 永远不该出现在提示里 —— 一天是赚不回来的。
func Reachable(list []Item, dailyRate float64) []Item {
	out := make([]Item, 0, len(list))
	for _, it := range list {
		if it.Price > 0 && it.Price <= dailyRate {
			out = append(out, it)
		}
	}
	return out
}

// Pending 在 Reachable 的基础上去掉已经买得起的物品，返回还够得着的那些。
//
// 这就是提示区的候选池：今天赚得到，并且现在还买不了。
func Pending(list []Item, earned, dailyRate float64) []Item {
	reachable := Reachable(list, dailyRate)
	out := make([]Item, 0, len(reachable))
	for _, it := range reachable {
		if it.Price > earned {
			out = append(out, it)
		}
	}
	return out
}
