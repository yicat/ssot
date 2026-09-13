/**
 * 词表测试夹具。
 *
 * 词表是**项目定义的中文名**。测试里给它一份最小的，形状与真实的一致：
 * 中文来自 schema 的 description、单位表、领域枚举，主体名来自数据。
 */
export function glossaryFixture() {
  const term = (key: string, label: string, note = "", extra = "") => ({ key, label, note, extra });
  return {
    entities: {
      shikigami: term("shikigami", "式神", "式神"),
      skill: term("skill", "技能", "技能"),
    },
    fields: {
      "shikigami.atk": term("shikigami.atk", "攻击", "攻击", "数值 · 点数（point）"),
      "shikigami.id": term("shikigami.id", "角色唯一 ID", "角色唯一 ID", "数值 · 点数（point）"),
      "shikigami.crit_factor": term(
        "shikigami.crit_factor",
        "期望暴击系数（= 1 + cri × crid，派生 L3）",
        "期望暴击系数（= 1 + cri × crid，派生 L3）",
        "数值 · 小数（fraction）",
      ),
      "skill.id": term("skill.id", "技能唯一 ID，形如 262_01", "技能唯一 ID，形如 262_01", "文本"),
      "skill.character_id": term("skill.character_id", "所属式神", "所属式神", "引用 · 指向 式神（shikigami）"),
      "skill.ratio": term("skill.ratio", "一级伤害倍率（从 description 文本抽取，L2）", "", "数值 · 百分比（percent）"),
      "skill.ratio_max": term("skill.ratio_max", "满级伤害倍率（从最后一条 upgrade 文本抽取，L2）", "", "数值 · 百分比（percent）"),
    },
    units: {
      fraction: term("fraction", "小数", "比值维度", "比值"),
      percent: term("percent", "百分比", "比值维度", "比值"),
      point: term("point", "点数", "点数维度", "点数"),
      min: term("min", "分钟", "时间维度", "时间"),
    },
    types: {
      text: term("text", "文本"),
      number: term("number", "数值"),
      bool: term("bool", "布尔"),
      enum: term("enum", "枚举"),
      ref: term("ref", "引用"),
    },
    subjects: {
      "shikigami/262": "姑获鸟",
      "skill/262_01": "伞剑",
      "skill/262_03": "天翔鹤斩",
    },
    confidences: {
      L1: term("L1", "直引", "可与原文逐字比对"),
      L2: term("L2", "结构化", "从文本解析得出"),
      L3: term("L3", "推导", "由其他断言算出"),
      L4: term("L4", "推断", "含补全的假设"),
    },
    statuses: {
      pending: term("pending", "待核验"),
      verified: term("verified", "已核验"),
      rejected: term("rejected", "已驳回"),
      disputed: term("disputed", "有争议"),
      "auto-checked": term("auto-checked", "已自动预检"),
    },
    actorKinds: { human: term("human", "人"), agent: term("agent", "机器") },
    expKinds: {
      derived: term("derived", "推导型"),
      judgment: term("judgment", "判定型"),
      summary: term("summary", "总结型"),
    },
    expStatuses: {
      candidate: term("candidate", "候选"),
      effective: term("effective", "生效"),
      rejected: term("rejected", "已驳回"),
      stale: term("stale", "失效"),
      recompute: term("recompute", "待重算"),
      superseded: term("superseded", "已被取代"),
    },
    decStatuses: {
      open: term("open", "待判定"),
      deferred: term("deferred", "已暂缓"),
      decided: term("decided", "已裁决"),
      stale: term("stale", "需复核"),
    },
    planStatuses: {
      candidate: term("candidate", "候选"),
      chosen: term("chosen", "已选中"),
      stale: term("stale", "依据已变"),
      invalid: term("invalid", "不可执行"),
      superseded: term("superseded", "已被取代"),
    },
  };
}

/** 词表接口的调用 ID（与 bindings 一致）。 */
export const GlossaryCallID = 2971284389;
