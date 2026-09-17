// onnx-probe —— 探「纯 Go（免 cgo）能不能加载 ONNX Runtime 跑我们的嵌入模型」。
//
// 做什么：用 `github.com/getcharzp/onnxruntime_purego` 加载 `onnxruntime.dll`，
// 给 bge-small-zh-v1.5（int8）建会话，喂一条构造好的 token 序列跑一次推理，
// 打印 ORT 版本、加载/推理耗时、输出的名字与形状。
//
// 为什么要有它：P1（嵌入）整条路都压在一个假设上——「本机没有 gcc，所以走 purego 加载 DLL，
// 而不是 cgo 链接 ORT」。这个假设要么被实测证实，要么早点推翻（推翻就得换方案）。
// 它同时也是**模型与运行时的可用性检查**：路径给错、DLL 版本不对、模型损坏，这里一眼能看出来。
//
// 什么时候不该用：
//   - 它不是嵌入实现，也不验嵌入质量——不比较向量、不对比 transformers.js；
//     那一层在 `internal/infrastructure/vembed`（P1）。
//   - 它**不含分词**：这里直接喂 token id。分词器是另一件事（tokenizer.json → Go）。
//   - 别拿它的耗时当性能结论：只跑了一条、没预热，量级参考而已（见 embedding-spike.md §六）。
//
// 用法（两个参数：DLL 路径、模型路径）：
//
//	go run ./scripts/check/onnx-probe <onnxruntime.dll> <model_quantized.onnx>
//
// 本机实测（2026-09-18）：
//
//	ORT 版本 1.30.0，加载 68ms；建会话 68ms；推理 1ms；
//	输出 last_hidden_state：[1 8 512]
package main

import (
	"fmt"
	"os"
	"time"

	ort "github.com/getcharzp/onnxruntime_purego"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("用法：go run ./scripts/check/onnx-probe <onnxruntime.dll> <model_quantized.onnx>")
		os.Exit(2)
	}
	dll, model := os.Args[1], os.Args[2]

	t0 := time.Now()
	eng, err := ort.NewEngine(dll)
	if err != nil {
		fmt.Println("NewEngine 失败:", err)
		os.Exit(1)
	}
	defer eng.Destroy()
	fmt.Printf("ORT 版本 %s，加载耗时 %v\n", eng.GetVersion(), time.Since(t0).Round(time.Millisecond))

	opts, err := eng.NewSessionOptions()
	if err != nil {
		fmt.Println("NewSessionOptions 失败:", err)
		os.Exit(1)
	}
	defer opts.Destroy()
	_ = opts.SetIntraOpNumThreads(2)

	t1 := time.Now()
	sess, err := eng.NewSession(model, opts)
	if err != nil {
		fmt.Println("NewSession 失败:", err)
		os.Exit(1)
	}
	defer sess.Destroy()
	fmt.Printf("会话建好，耗时 %v\n", time.Since(t1).Round(time.Millisecond))

	// bge 系列是 BERT 结构：输入名就这三个（这个绑定没导出「取输入名」的方法，所以写死）。
	ids := []int64{101, 704, 5679, 2523, 102, 0, 0, 0} // [CLS] 伤害 计算 [SEP] + padding
	mask := []int64{1, 1, 1, 1, 1, 0, 0, 0}
	types := []int64{0, 0, 0, 0, 0, 0, 0, 0}
	shape := []int64{1, int64(len(ids))}

	mk := func(name string, data []int64) *ort.Value {
		v, err := ort.NewTensor(shape, data)
		if err != nil {
			fmt.Printf("NewTensor(%s) 失败: %v\n", name, err)
			os.Exit(1)
		}
		return v
	}
	in := map[string]*ort.Value{
		"input_ids":      mk("input_ids", ids),
		"attention_mask": mk("attention_mask", mask),
		"token_type_ids": mk("token_type_ids", types),
	}
	for _, v := range in {
		defer v.Destroy()
	}

	t2 := time.Now()
	out, err := sess.Run(in)
	if err != nil {
		fmt.Println("Run 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("推理完成，耗时 %v，输出 %d 个\n", time.Since(t2).Round(time.Millisecond), len(out))

	for name, v := range out {
		sh, err := v.GetShape()
		if err != nil {
			fmt.Printf("  输出 %s：取形状失败 %v\n", name, err)
			continue
		}
		data, err := ort.GetTensorData[float32](v)
		if err != nil {
			fmt.Printf("  输出 %s：形状 %v，取数据失败 %v\n", name, sh, err)
			continue
		}
		head := data
		if len(head) > 4 {
			head = head[:4]
		}
		fmt.Printf("  输出 %s：形状 %v，共 %d 个数，前几个 %v\n", name, sh, len(data), head)
	}
}
