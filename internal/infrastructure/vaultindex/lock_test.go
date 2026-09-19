package vaultindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIndexLock 是索引互斥的验收：第二个进程要拿到**人话**错误，而不是「文件忙」。
func TestIndexLock(t *testing.T) {
	root := t.TempDir()
	idx := New(root)
	unlock, err := idx.lock()
	if err != nil {
		t.Fatal(err)
	}
	// 另一个 Index（模拟另一个进程）来抢：该被挡住并说清原因。
	other := New(root)
	if _, err := other.lock(); err == nil {
		t.Fatal("同一份 vault 该只允许一个进程拿锁")
	} else if !strings.Contains(err.Error(), "正在重建索引") {
		t.Errorf("报错要说人话（有人在建索引），实际：%v", err)
	}
	unlock()
	// 释放之后能抢到。
	unlock2, err := idx.lock()
	if err != nil {
		t.Fatalf("释放后该抢得到：%v", err)
	}
	unlock2()

	// 老锁当死锁：清掉重抢（上一个进程死了不该永久卡住）。
	p := idx.lockPath()
	if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	unlock3, err := idx.lock()
	if err != nil {
		t.Fatalf("老锁该被当作死锁清掉：%v", err)
	}
	unlock3()
	_ = filepath.Join
}
