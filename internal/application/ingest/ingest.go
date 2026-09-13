// Package ingest 是接入层：把原件解析成候选。
//
// 见 docs/specs/ingestion.spec.md。接入层**只负责"拿到"与"看懂格式"**，
// 不判断内容是否为事实、不决定可信度——那是准入层的事。
//
// 候选携带「解析方式」，供准入层据此定级：回退或推测解析**不得定为 L1**。
package ingest

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/value"
)

// Parsing 是候选的解析方式。
//
// 它是 assertion.Parsing 的别名：**「回退解析不得定为 L1」是领域规则**，
// 定义在 domain 层；接入层只负责如实标注自己是怎么读出来的。
type Parsing = assertion.Parsing

const (
	// ParsingDirect 结构化直取：字段就在结构化数据里，可逐字比对 → L1
	ParsingDirect = assertion.ParsingDirect
	// ParsingText 从文本解析得出 → L2
	ParsingText = assertion.ParsingText
	// ParsingFallback 回退或推测解析 → **不得定为 L1**
	ParsingFallback = assertion.ParsingFallback
)

// Candidate 是一个候选：值 + 命中位置 + 解析方式。
// 它**不是断言**——还没有经过准入。
type Candidate struct {
	Entity     string
	Subject    string
	Predicate  string
	Value      value.Value
	Qualifiers assertion.Qualifiers

	Artifact string
	Anchor   string
	// Context 是该候选周围的原文片段。
	//
	// 只有歧义候选需要它：唯一命中时锚点已足够定位；而人要在几个候选间取舍时，
	// 只给一个数字他无法判断。领域层要求「候选必须带上下文」，此处照实收集。
	Context  string
	Revision string

	Parsing Parsing
}

// 注意：MaxConfidence 定义在 domain/assertion 上。因为 Parsing 是别名，
// 别名类型上**不允许**再定义方法（非本地类型），所以这里只做类型转发。

// attributeRecord 是 Data:Attribute.json 中的一条。
//
// 实测结构（v1.58 数据，revid 9939）：
//
//	{"id":262,"name":"姑获鸟","rarity":"SR","atk":3082,"hp":10823,"def":397,
//	 "spd":113,"cri":0.5,"crid":1.2,"efh":0,"efr":0}
//
// 注意量纲：`cri`/`crid` 是**小数**（0.5 = 50%），而另一个来源
// Data:Character/<id>.json 用的是百分数（crit.value = 50）。
// 两者归一后一致——量纲表负责这件事。
type attributeRecord struct {
	ID     json.Number `json:"id"`
	Name   string      `json:"name"`
	Rarity string      `json:"rarity"`
	Atk    json.Number `json:"atk"`
	Hp     json.Number `json:"hp"`
	Def    json.Number `json:"def"`
	Spd    json.Number `json:"spd"`
	Cri    json.Number `json:"cri"`
	Crid   json.Number `json:"crid"`
	Efh    json.Number `json:"efh"`
	Efr    json.Number `json:"efr"`
}

type attributeFile struct {
	Attributes []attributeRecord `json:"attributes"`
}

// 字段 → 量纲的映射。数值字段必须声明单位（见 metamodel.spec.md）。
var attributeUnits = map[string]string{
	"atk":  "point",
	"hp":   "point",
	"def":  "point",
	"spd":  "point",
	"cri":  "fraction",
	"crid": "fraction",
	"efh":  "fraction",
	"efr":  "fraction",
}

// AttributeJSON 把 Data:Attribute.json 解析成候选。
//
// 全部字段都来自结构化 JSON 直取，因此解析方式为 direct（→ 最高 L1）。
func AttributeJSON(artifactName string, body []byte, revision string, capturedAt time.Time) ([]Candidate, error) {
	var f attributeFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", artifactName, err)
	}
	if len(f.Attributes) == 0 {
		return nil, fmt.Errorf("%s 中没有 attributes", artifactName)
	}

	var out []Candidate
	add := func(subject, predicate string, v value.Value, anchor string) {
		out = append(out, Candidate{
			Entity: "shikigami", Subject: subject, Predicate: predicate, Value: v,
			Artifact: artifactName, Anchor: anchor, Revision: revision, Parsing: ParsingDirect,
		})
	}

	for i, r := range f.Attributes {
		id := r.ID.String()
		if id == "" {
			return nil, fmt.Errorf("%s：第 %d 条缺少 id", artifactName, i)
		}
		idNum, err := r.ID.Float64()
		if err != nil {
			return nil, fmt.Errorf("%s：第 %d 条的 id 不是数值：%q", artifactName, i, id)
		}
		base := fmt.Sprintf("attributes[%d]", i)

		// 身份字段本身也是一条断言
		add(id, "id", value.OfUnit(idNum, "point"), base+".id")
		add(id, "name", value.Of(r.Name), base+".name")
		if r.Rarity != "" {
			add(id, "rarity", value.Of(r.Rarity), base+".rarity")
		}
		for field, num := range map[string]json.Number{
			"atk": r.Atk, "hp": r.Hp, "def": r.Def, "spd": r.Spd,
			"cri": r.Cri, "crid": r.Crid, "efh": r.Efh, "efr": r.Efr,
		} {
			if num.String() == "" {
				continue
			}
			n, err := num.Float64()
			if err != nil {
				return nil, fmt.Errorf("%s：%s.%s 不是数值：%q", artifactName, id, field, num.String())
			}
			add(id, field, value.OfUnit(n, attributeUnits[field]), base+"."+field)
		}
	}
	return out, nil
}

// indexRecord 是 Data:CharacterIndex.json 中的一条（用于别名登记，MVP 暂不做）。
type indexRecord struct {
	ID       json.Number `json:"id"`
	Name     string      `json:"name"`
	Rarity   string      `json:"rarity"`
	Nickname []string    `json:"nickname"`
}

type indexFile struct {
	Characters []indexRecord `json:"characters"`
}

// IndexNames 返回索引中各角色的名字，供穷尽性检查使用。
func IndexNames(body []byte) (map[string]string, error) {
	var f indexFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("解析索引失败：%w", err)
	}
	out := map[string]string{}
	for _, c := range f.Characters {
		out[c.ID.String()] = c.Name
	}
	return out, nil
}
