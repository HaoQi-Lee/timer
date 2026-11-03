// Package goods 内置用于"赚回来"估算的常见物品与价格。
package goods

// Item 是一件物品及其价格，Name 里带上量词方便直接拼成一句话。
type Item struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Emoji string  `json:"emoji"`
}

// Builtin 是内置的物品清单，从十几块的小零食一路排到五千块的大家电，
// 价格在整个区间里大致均匀分布 —— 这样不管日薪多少，
// 总能找到"再干一会儿就买得起"的那一件。
//
// 名称都按「X 就赚回来了」这句话读起来顺当来取名，比如"理一次发"而不是"一次理发"。
var Builtin = []Item{
	// 10–100 元
	{Name: "一个面包", Price: 12, Emoji: "🥐"},
	{Name: "一杯奶茶", Price: 18, Emoji: "🧋"},
	{Name: "一杯咖啡", Price: 26, Emoji: "☕"},
	{Name: "打车一次", Price: 38, Emoji: "🚕"},
	{Name: "点一份外卖", Price: 55, Emoji: "🍱"},
	{Name: "看场电影", Price: 75, Emoji: "🎬"},
	{Name: "理一次发", Price: 95, Emoji: "💈"},

	// 100–500 元
	{Name: "洗一次车", Price: 120, Emoji: "🚿"},
	{Name: "充一次话费", Price: 150, Emoji: "📱"},
	{Name: "买一双拖鞋", Price: 180, Emoji: "🩴"},
	{Name: "一件 T恤", Price: 220, Emoji: "👕"},
	{Name: "做一次美甲", Price: 260, Emoji: "💅"},
	{Name: "一顿火锅", Price: 300, Emoji: "🍲"},
	{Name: "做一次按摩", Price: 350, Emoji: "💆"},
	{Name: "一支口红", Price: 400, Emoji: "💄"},
	{Name: "一双运动鞋", Price: 450, Emoji: "👟"},

	// 500–1000 元
	{Name: "一次私教课", Price: 520, Emoji: "🏋️"},
	{Name: "一张演唱会门票", Price: 590, Emoji: "🎤"},
	{Name: "一件外套", Price: 660, Emoji: "🧥"},
	{Name: "一顿自助餐", Price: 740, Emoji: "🥩"},
	{Name: "一把电动剃须刀", Price: 820, Emoji: "🪒"},
	{Name: "一次年度体检", Price: 900, Emoji: "🩺"},
	{Name: "一套护肤品", Price: 990, Emoji: "🧴"},

	// 1000–2000 元
	{Name: "一台电饭煲", Price: 1080, Emoji: "🍚"},
	{Name: "一副蓝牙耳机", Price: 1180, Emoji: "🎧"},
	{Name: "一台显示器", Price: 1299, Emoji: "🖥️"},
	{Name: "一次洗牙", Price: 1399, Emoji: "🦷"},
	{Name: "一把人体工学椅", Price: 1499, Emoji: "🪑"},
	{Name: "一台打印机", Price: 1599, Emoji: "🖨️"},
	{Name: "一箱进口红酒", Price: 1699, Emoji: "🍷"},
	{Name: "一台游戏机", Price: 1799, Emoji: "🎮"},
	{Name: "一台扫地机器人", Price: 1899, Emoji: "🤖"},

	// 2000–3000 元
	{Name: "一双跑鞋", Price: 1999, Emoji: "🏃"},
	{Name: "一台咖啡机", Price: 2099, Emoji: "🫘"},
	{Name: "一辆自行车", Price: 2199, Emoji: "🚲"},
	{Name: "一台净水器", Price: 2299, Emoji: "💧"},
	{Name: "一套家庭影音", Price: 2399, Emoji: "🎞️"},
	{Name: "一台洗碗机", Price: 2499, Emoji: "🍽️"},
	{Name: "一套健身器材", Price: 2699, Emoji: "💪"},
	{Name: "一台空气净化器", Price: 2899, Emoji: "🌬️"},

	// 3000–5000 元
	{Name: "一次家装咨询", Price: 3099, Emoji: "🏠"},
	{Name: "一台笔记本电脑", Price: 3299, Emoji: "💻"},
	{Name: "一场国内旅行", Price: 3599, Emoji: "✈️"},
	{Name: "一台相机", Price: 3999, Emoji: "📷"},
	{Name: "一套智能家居", Price: 4499, Emoji: "🔌"},
	{Name: "一次家装升级", Price: 4999, Emoji: "🏗️"},
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
