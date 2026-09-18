package vault

// ExtractStat 是派生图（实体 / 关系）的家底。
//
// 为什么在 domain：它是「水面下有什么」的口径，换掉 SQLite 也还是这几个数；
// 界面与 CLI 都要显示它（P5 的「索引状态」）。
type ExtractStat struct {
	Entities     int // 来源行数（同一实体多来源就是多行）
	EntityNames  int // 去重后的实体数（按 (name,type)）
	Relations    int // 关系来源行数
	RelationKeys int // 去重后的关系数（按 (src,dst,keywords)）
	Docs         int // 有产物的文档数
	Corrected    int // 其中标了「人纠错过」的行数
}
