// Package document 是文档的用例编排。
//
// 见 docs/specs/document.spec.md。规则本身在 domain/document 里（纯函数、可测）；
// 本包负责一件必须碰数据的事：**变更传导**——文档的当前修订变了，
// 引用它的断言要回到待核验。
//
// 这条规则 core.spec.md 早就写了，此前一直做不到，因为文档只是溯源里的一个字符串，
// 系统不知道它现在的修订是什么。
package document

import (
	"fmt"
	"time"

	"github.com/ngnl5/ssot/internal/domain/assertion"
	"github.com/ngnl5/ssot/internal/domain/document"
	"github.com/ngnl5/ssot/internal/domain/verification"
)

// Port 是文档所需的读写端口。
type Port interface {
	PutDocument(document.Doc) error
	MarkDocumentSuperseded(revID, by string) error
	UpdateDocumentStatus(revID, status, reason string, by *verification.Actor, method verification.Method) error
	Documents(scenario string) ([]document.Doc, error)
	DocumentByRev(revID string) (document.Doc, bool, error)
	DocumentHistory(docID string) ([]document.Doc, error)
	FindDocument(source, title string) (document.Doc, bool, error)

	// RequeueByArtifact 把引用某原件的断言回到待核验，返回条数。
	RequeueByArtifact(title, currentRevision string, force bool) (int, error)
	AssertionsByArtifact(title string) ([]assertion.Assertion, error)
	UnregisteredArtifacts() ([]document.UnregisteredRef, error)
}

// Result 是一次登记的结果。
type Result struct {
	Doc document.Doc
	// Changed 报告当前修订是否真的变了。
	Changed bool
	// Reverted 报告修订标识回退到旧值——这通常意味着来源出过问题。
	Reverted bool
	// Requeued 是本次因修订变化回到待核验的断言数。
	//
	// **必须在界面上说出来**：一次同步让几百条已核验的断言回到待核验，
	// 而人不被告知，就是最坏的那种静默。
	Requeued int
}

// Register 登记一份文档（可能是新修订）。
//
// 幂等：同一份修订重复登记不产生变更，也不动断言。
func Register(p Port, in document.Input, now time.Time) (Result, error) {
	next, err := document.New(in, now)
	if err != nil {
		return Result{}, err
	}
	history, err := p.DocumentHistory(next.ID)
	if err != nil {
		return Result{}, err
	}

	// 首次登记：这是一份新文档，不是一次变更——**不动任何断言**
	if len(history) == 0 {
		if err := p.PutDocument(next); err != nil {
			return Result{}, err
		}
		return Result{Doc: next, Changed: true}, nil
	}

	_, ch, err := document.Apply(history, next)
	if err != nil {
		return Result{}, err
	}
	if !ch.Changed {
		return Result{Doc: ch.Doc}, nil
	}

	if err := p.PutDocument(next); err != nil {
		return Result{}, err
	}
	res := Result{Doc: next, Changed: true, Reverted: ch.Reverted}
	if ch.Previous == nil {
		return res, nil
	}
	if err := p.MarkDocumentSuperseded(ch.Previous.RevID, next.RevID); err != nil {
		return res, err
	}

	// 变更传导：引用这个原件的断言回到待核验。
	//
	// 修订标识没变而内容变了时（force），无法分辨哪些断言是从旧内容抽出来的，
	// 只能整个原件上的断言全部回退——宁可多核验一次。
	force := ch.Previous.Revision == next.Revision
	n, err := p.RequeueByArtifact(next.Title, next.Revision, force)
	if err != nil {
		return res, err
	}
	res.Requeued = n
	return res, nil
}

// Verify 为这份原文背书。核验人必须是人，方法只能是编审。
func Verify(p Port, revID string, by verification.Actor, method verification.Method,
	reason string) (document.Doc, error) {

	d, ok, err := p.DocumentByRev(revID)
	if err != nil {
		return document.Doc{}, err
	}
	if !ok {
		return document.Doc{}, fmt.Errorf("文档修订 %s 不存在", revID)
	}
	if method == "" {
		method = verification.Editorial
	}
	next, err := d.Verify(by, method, reason)
	if err != nil {
		return document.Doc{}, err
	}
	byCopy := by
	if err := p.UpdateDocumentStatus(revID, string(next.Status), next.Reason, &byCopy, next.Method); err != nil {
		return document.Doc{}, err
	}
	return next, nil
}

// Reject 驳回一份文档。条目保留——驳回是审计轨迹。
func Reject(p Port, revID string, by verification.Actor, reason string) (document.Doc, error) {
	d, ok, err := p.DocumentByRev(revID)
	if err != nil {
		return document.Doc{}, err
	}
	if !ok {
		return document.Doc{}, fmt.Errorf("文档修订 %s 不存在", revID)
	}
	next, err := d.Reject(by, reason)
	if err != nil {
		return document.Doc{}, err
	}
	byCopy := by
	if err := p.UpdateDocumentStatus(revID, string(next.Status), next.Reason, &byCopy, next.Method); err != nil {
		return document.Doc{}, err
	}
	return next, nil
}

// Usage 返回引用某份文档的断言。
//
// 关联方式是**标题**：断言的溯源里 `Artifact` 就是文档的标题。
func Usage(p Port, revID string) ([]assertion.Assertion, error) {
	d, ok, err := p.DocumentByRev(revID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("文档修订 %s 不存在", revID)
	}
	return p.AssertionsByArtifact(d.Title)
}

// Unregistered 返回还没有对应文档的原件。
func Unregistered(p Port) ([]document.UnregisteredRef, error) {
	return p.UnregisteredArtifacts()
}
