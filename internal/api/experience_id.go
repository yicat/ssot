package api

import (
	"crypto/sha1"
	"encoding/hex"
	"time"
)

// sessionID 由 (场景, 标题, 时刻) 决定。
//
// 精确到纳秒：同一秒内开两个同名会话会撞 ID，而那在界面上只是「点一下」的事。
// 同一次调用必然得到同一个 ID，因此重复点也不会写出两条。
func sessionID(scenario, title string, at time.Time) string {
	h := sha1.Sum([]byte(scenario + "|" + title + "|" + at.Format(time.RFC3339Nano)))
	return "s" + hex.EncodeToString(h[:8])
}
