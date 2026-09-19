// 索引的**互斥**：同一份 vault 只允许一个进程在重建/写索引。
//
// 为什么需要（实测踩过）：一个进程在 Rebuild（它先删 index.db），另一个进程也要重建，
// 后来者会拿到 `The process cannot access the file ... index.db` —— 那句报错完全看不出
// 「有人在建索引」。这里用最朴素的办法：`.data/index.lock` 抢创建；抢不到就**说人话**，
// 而且锁太老（>10 分钟）时认为上一个进程死了，清掉重抢。
package vaultindex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockStaleAfter = 10 * time.Minute

// lockPath 是锁文件的位置。
func (idx *Index) lockPath() string { return filepath.Join(idx.root, ".data", "index.lock") }

// lock 抢锁；返回释放函数。
func (idx *Index) lock() (func(), error) {
	p := idx.lockPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	if err := idx.tryLock(p); err != nil {
		// 抢不到：老锁当死锁清掉，重抢一次；还不行就报人话错误。
		if stale, why := idx.lockIsStale(p); stale {
			_ = os.Remove(p)
			if err2 := idx.tryLock(p); err2 == nil {
				return idx.releaseFunc(p), nil
			}
		} else if why != "" {
			return nil, fmt.Errorf("另一道进程正在重建索引（%s）：%s；等它跑完再试", p, why)
		}
		_ = os.Remove(p)
		if err2 := idx.tryLock(p); err2 != nil {
			return nil, fmt.Errorf("抢索引锁失败（%s）：%w", p, err2)
		}
	}
	return idx.releaseFunc(p), nil
}

func (idx *Index) tryLock(p string) error {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	// 写上是谁、什么时候抢的：出问题时这句就是线索。
	info := map[string]any{"pid": os.Getpid(), "at": time.Now().Format(time.RFC3339)}
	b, _ := json.Marshal(info)
	_, _ = f.Write(b)
	return nil
}

func (idx *Index) releaseFunc(p string) func() {
	return func() { _ = os.Remove(p) }
}

// lockIsStale 判断锁是不是死的（老到不可能还在建索引），并给出原因。
func (idx *Index) lockIsStale(p string) (bool, string) {
	fi, err := os.Stat(p)
	if err != nil {
		return true, "锁文件读不到"
	}
	age := time.Since(fi.ModTime())
	if age > lockStaleAfter {
		return true, fmt.Sprintf("锁已存在 %s（超过 %s，当作上一个进程死了）", age.Round(time.Second), lockStaleAfter)
	}
	body, _ := os.ReadFile(p)
	return false, fmt.Sprintf("锁文件 %s，内容 %s", age.Round(time.Second), string(body))
}
