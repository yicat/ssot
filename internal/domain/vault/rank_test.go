package vault

import "testing"

func TestOverlap2Gram(t *testing.T) {
	cases := []struct {
		name string
		q    string
		hay  string
		want float64
		tol  float64
	}{
		// ⚠️ 已知偏差：问句后缀（「是什么」）也算进分母，于是实体名完全命中时也只有 0.5。
		// 这不是 bug，是这套度量的性质——它摊薄了标题项的加成，所以 β 要够大才看得见效果。
		{"问句后缀会摊薄重合率", "茨木童子 是什么？", "茨木童子 原始层,剧情", 0.5, 1e-9},
		// 实体名只是查询的一部分：命中一半的 gram（另一半是「的技能」）。
		{"实体名只是一部分", "茨木童子的技能", "茨木童子", 0.5, 1e-9},
		{"实体名整串命中且查询就是实体名", "茨木童子", "茨木童子 原始层,剧情", 1, 1e-9},
		{"完全不搭", "伤害计算", "茨木童子", 0, 1e-9},
		{"标点被清掉", "伤害 = 攻击 × 系数", "伤害攻击系数", 1, 1e-9},
		{"查询太短无从比较", "好", "好用的东西", 0, 1e-9},
		{"目标为空", "伤害计算", "", 0, 1e-9},
	}
	for _, c := range cases {
		got := Overlap2Gram(c.q, c.hay)
		if d := got - c.want; d > c.tol || d < -c.tol {
			t.Errorf("%s：Overlap2Gram(%q, %q) = %.3f，想要 %.3f", c.name, c.q, c.hay, got, c.want)
		}
	}
}

func TestFuseHybrid(t *testing.T) {
	w := DefaultHybridWeights()
	if got := FuseHybrid(0.5, 0, 0, w); got != 0.5 {
		t.Errorf("字面项为 0 时分数不该变：%v", got)
	}
	if got := FuseHybrid(0.5, 1, 0, w); got != 0.5+w.TitleTags {
		t.Errorf("标题项该按权重加上去：%v", got)
	}
	// 权重为 0（纯向量）时字面项一律不参与。
	if got := FuseHybrid(0.5, 1, 1, HybridWeights{}); got != 0.5 {
		t.Errorf("零权重该等于纯向量：%v", got)
	}
	// 字面项只是**加成**，不该压过向量（权重上限远小于 1）。
	if w.TitleTags >= 1 {
		t.Errorf("标题项权重 %v 太大：字面项会盖过语义", w.TitleTags)
	}
}
