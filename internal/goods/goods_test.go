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

func TestShuffledIsDeterministic(t *testing.T) {
	a := Shuffled(Builtin, 20261103)
	b := Shuffled(Builtin, 20261103)
	if len(a) != len(Builtin) {
		t.Fatalf("洗牌后长度应不变，实际 %d", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("同一种子应得到同样的顺序，第 %d 项不同", i)
		}
	}
}

func TestShuffledKeepsAllItems(t *testing.T) {
	got := Shuffled(Builtin, 1)
	seen := map[string]int{}
	for _, it := range got {
		seen[it.Name]++
	}
	for _, it := range Builtin {
		if seen[it.Name] != 1 {
			t.Errorf("物品 %s 应恰好出现一次，实际 %d 次", it.Name, seen[it.Name])
		}
	}
}

func TestShuffledDoesNotMutateInput(t *testing.T) {
	before := Builtin[0]
	_ = Shuffled(Builtin, 7)
	if Builtin[0] != before {
		t.Error("洗牌不应改动原切片")
	}
}
