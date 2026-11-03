package goods

import "testing"

func TestBuiltinHasTShirt(t *testing.T) {
	for _, it := range Builtin {
		if it.Name == "一件 T恤" {
			if it.Price != 99 {
				t.Errorf("T 恤价格应为 99，实际 %v", it.Price)
			}
			return
		}
	}
	t.Fatal("内置清单里应有一件 T恤")
}

func TestReachableOnlyKeepsItemsWithinOneDay(t *testing.T) {
	// 日薪 200 的话，一天只够买下 200 以内的东西。
	got := Reachable(Builtin, 200)
	for _, it := range got {
		if it.Price > 200 {
			t.Errorf("%s 价格 %v 超过日薪，不该算今天能赚到", it.Name, it.Price)
		}
	}
	if len(got) == 0 {
		t.Fatal("日薪 200 时应有若干物品是今天赚得到的")
	}
	if len(got) == len(Builtin) {
		t.Error("日薪 200 时不该把几千块的东西也算进去")
	}
}

func TestReachableKeepsEverythingWhenDailyRateIsHuge(t *testing.T) {
	got := Reachable(Builtin, 100000)
	if len(got) != len(Builtin) {
		t.Errorf("日薪足够高时应保留全部物品，实际 %d/%d", len(got), len(Builtin))
	}
}

func TestPendingDropsItemsAlreadyAffordable(t *testing.T) {
	// 已赚 60 元：奶茶、面包、咖啡、打车、外卖、电影、理发都买得起了。
	got := Pending(Builtin, 60, 200)
	for _, it := range got {
		if it.Price <= 60 {
			t.Errorf("%s 只要 %v 元，已经买得起，不该再提示", it.Name, it.Price)
		}
	}
	if len(got) == 0 {
		t.Fatal("还有 T恤、鞋子之类买得起的，应该留一件")
	}
}

func TestPendingIsEmptyWhenEverythingIsAffordable(t *testing.T) {
	got := Pending(Builtin, 100000, 100000)
	if len(got) != 0 {
		t.Errorf("全都买得起时候选池应为空，实际 %d", len(got))
	}
}

func TestPendingOnlyKeepsItemsReachableToday(t *testing.T) {
	got := Pending(Builtin, 0, 200)
	for _, it := range got {
		if it.Price > 200 {
			t.Errorf("%s 一天赚不到，不该进入候选池", it.Name)
		}
	}
}
