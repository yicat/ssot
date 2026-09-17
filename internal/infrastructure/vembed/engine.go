package vembed

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	ort "github.com/getcharzp/onnxruntime_purego"
)

// Options 是打开嵌入引擎需要的三样东西。
type Options struct {
	// ModelDir 是模型目录：`model.onnx` + `tokenizer.json`（`onnxruntime.dll` 也放这儿最省事）。
	ModelDir string
	// DLLPath 是 ONNX 运行时；留空就用 `<ModelDir>/onnxruntime.dll`。
	DLLPath string
	// Threads 是推理线程数（<=0 时用 2）。
	Threads int
}

// Engine 是「文本 → 向量」的运行时：分词器 + ONNX 会话。
//
// ⚠️ 它**不是并发安全**的（ONNX 会话每次 Run 会复用内部 buffer）。要并发就开多个 Engine。
type Engine struct {
	eng  *ort.Engine
	sess *ort.Session
	tok  *Tokenizer
	dim  int
	// withTypeIDs 记录这个模型要不要喂 `token_type_ids`：
	// 这个绑定**没有导出「取输入名」的方法**（`getInputName` 是小写），只能试一次记住结果。
	// 默认 true：BERT 系列的 ONNX 导出（含 bge）都要它，实测缺了会报 Missing Input。
	withTypeIDs bool
}

// DefaultModelDir 找模型目录：先看 `SSOT_EMBED_DIR`，再看用户配置目录下的 `ssot/models/bge-small-zh-v1.5`。
//
// 返回空串表示没找到——调用方要给**人话**错误，而不是静默退化（见 docs/OPEN.md #15：
// 模型与运行时怎么分发还没定，所以现在只能由环境或配置指路）。
func DefaultModelDir(configDir string) string {
	if d := os.Getenv("SSOT_EMBED_DIR"); d != "" {
		return d
	}
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, "ssot", "models", "bge-small-zh-v1.5")
}

// Open 装载分词器与模型。
//
// 失败要说清缺什么：模型目录、DLL、模型文件都可能缺，报错里直接给路径。
func Open(opts Options) (*Engine, error) {
	if opts.ModelDir == "" {
		return nil, fmt.Errorf("没给模型目录：设 SSOT_EMBED_DIR，或把模型放到 <配置目录>/ssot/models/bge-small-zh-v1.5")
	}
	dll := opts.DLLPath
	if dll == "" {
		dll = filepath.Join(opts.ModelDir, "onnxruntime.dll")
	}
	modelPath := filepath.Join(opts.ModelDir, "model.onnx")
	tokPath := filepath.Join(opts.ModelDir, "tokenizer.json")
	for _, p := range []string{dll, modelPath, tokPath} {
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("模型目录 %s 里缺东西（%s）：%w", opts.ModelDir, filepath.Base(p), err)
		}
	}

	tok, err := LoadTokenizer(tokPath)
	if err != nil {
		return nil, err
	}
	eng, err := ort.NewEngine(dll)
	if err != nil {
		return nil, fmt.Errorf("加载 ONNX 运行时 %s 失败：%w", dll, err)
	}
	sopts, err := eng.NewSessionOptions()
	if err != nil {
		eng.Destroy()
		return nil, err
	}
	threads := opts.Threads
	if threads <= 0 {
		threads = 2
	}
	_ = sopts.SetIntraOpNumThreads(int32(threads))
	sess, err := eng.NewSession(modelPath, sopts)
	sopts.Destroy()
	if err != nil {
		eng.Destroy()
		return nil, fmt.Errorf("装载模型 %s 失败：%w", modelPath, err)
	}

	e := &Engine{eng: eng, sess: sess, tok: tok, dim: readHiddenSize(opts.ModelDir), withTypeIDs: true}

	// 探一次：既定下要不要 token_type_ids，也确认整条路是通的（早失败好过几百次之后失败）。
	probeID, probeMask := tok.Encode("探")
	if _, _, err := e.run(probeID, probeMask); err != nil {
		e.Close()
		return nil, fmt.Errorf("试跑一次失败（模型/运行时对不上？）：%w", err)
	}
	return e, nil
}

// Close 释放会话与运行时。
func (e *Engine) Close() {
	if e.sess != nil {
		e.sess.Destroy()
		e.sess = nil
	}
	if e.eng != nil {
		e.eng.Destroy()
		e.eng = nil
	}
}

// Dim 是向量维度（模型加载后就有；读不到 config.json 时为 0，跑完第一条就补上）。
func (e *Engine) Dim() int { return e.dim }

// Tokenizer 暴露分词器（测试与调试用）。
func (e *Engine) Tokenizer() *Tokenizer { return e.tok }

// Embed 把一条文本变成单位向量：CLS 池化（取第一个 token 的隐状态）+ L2 归一。
//
// 我们的口径见 docs/specs/derived.spec.md：**CLS 池化 + L2 归一**，向量存 float32。
func (e *Engine) Embed(text string) ([]float32, error) {
	ids, mask := e.tok.Encode(text)
	if len(ids) == 0 {
		return nil, fmt.Errorf("空文本分不出 token（连 [CLS]/[SEP] 都没有）")
	}
	hidden, shape, err := e.run(ids, mask)
	if err != nil {
		return nil, err
	}
	if len(shape) != 3 || shape[0] != 1 {
		return nil, fmt.Errorf("模型输出形状不是 [1, seq, dim]：%v", shape)
	}
	dim := int(shape[2])
	if dim <= 0 || len(hidden) < dim {
		return nil, fmt.Errorf("模型输出不够一个向量：形状 %v，数据 %d 个", shape, len(hidden))
	}
	e.dim = dim
	vec := make([]float32, dim)
	copy(vec, hidden[:dim]) // CLS = 第一个 token
	normalizeL2(vec)
	return vec, nil
}

// run 跑一次推理，返回（隐状态, 形状, 错误）。
func (e *Engine) run(ids, mask []int64) ([]float32, []int64, error) {
	hidden, shape, err := e.runWith(ids, mask, e.withTypeIDs)
	if err == nil {
		return hidden, shape, nil
	}
	firstErr := err
	// 模型可能只要两个输入：反过来再试一次，并把结论记住（只在这条路上试一次）。
	hidden, shape, err = e.runWith(ids, mask, !e.withTypeIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("%w（换一种输入组合也不行：%v）", firstErr, err)
	}
	e.withTypeIDs = !e.withTypeIDs
	return hidden, shape, nil
}

func (e *Engine) runWith(ids, mask []int64, withTypeIDs bool) ([]float32, []int64, error) {
	shape := []int64{1, int64(len(ids))}
	in := map[string]*ort.Value{}
	defer func() {
		for _, v := range in {
			v.Destroy()
		}
	}()

	idv, err := ort.NewTensor(shape, ids)
	if err != nil {
		return nil, nil, err
	}
	in["input_ids"] = idv
	mv, err := ort.NewTensor(shape, mask)
	if err != nil {
		return nil, nil, err
	}
	in["attention_mask"] = mv
	if withTypeIDs {
		types := make([]int64, len(ids))
		tv, err := ort.NewTensor(shape, types)
		if err != nil {
			return nil, nil, err
		}
		in["token_type_ids"] = tv
	}

	out, err := e.sess.Run(in)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		for _, v := range out {
			v.Destroy()
		}
	}()
	var hidden []float32
	var outShape []int64
	for _, v := range out {
		data, err := ort.GetTensorData[float32](v)
		if err != nil {
			continue // 量化模型可能有别的输出（例如 int8 的附加输出），只要浮点那个
		}
		sh, err := v.GetShape()
		if err != nil {
			continue
		}
		hidden, outShape = data, sh
		break
	}
	if hidden == nil {
		return nil, nil, fmt.Errorf("模型输出里没有 float32 张量")
	}
	return hidden, outShape, nil
}

// normalizeL2 把向量变成单位向量（零向量原样返回，免得除零变成 NaN）。
func normalizeL2(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	s := math.Sqrt(sum)
	for i, x := range v {
		v[i] = float32(float64(x) / s)
	}
}

// readHiddenSize 从 config.json 里读维度（读不到就返回 0，跑一条再补上）。
func readHiddenSize(dir string) int {
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return 0
	}
	var cfg struct {
		HiddenSize int `json:"hidden_size"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return 0
	}
	return cfg.HiddenSize
}
