package main

import "testing"

// 检索结果给人看的那两行：写死它，免得「截断了却不说」这种事再回来
// （#31/#33 的教训：模型和人都是靠这句话判断拿全了没有）。
func TestSearchSummary(t *testing.T) {
	cases := []struct {
		name     string
		total    int
		returned int
		want     string
	}{
		{
			"拿全了：只说命中与返回",
			70, 70,
			"命中 70 篇，返回 70 条\n",
		},
		{
			"被截断：要明说还差多少、怎么办",
			70, 5,
			"命中 70 篇，返回 5 条\n（还有 65 篇没显示：用 -limit 调大）\n",
		},
		{
			"返回比命中还多（不该发生，但不能写出负数）",
			3, 5,
			"命中 3 篇，返回 5 条\n",
		},
		{
			"一篇都没命中",
			0, 0,
			"命中 0 篇，返回 0 条\n",
		},
	}
	for _, c := range cases {
		if got := searchSummary(c.total, c.returned); got != c.want {
			t.Errorf("%s：\n拿到 %q\n想要 %q", c.name, got, c.want)
		}
	}
}
