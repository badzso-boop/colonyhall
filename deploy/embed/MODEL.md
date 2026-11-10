# Models for the search (round 36)

The model files are not in the repository. Download them on the machine that
serves them, check the sha256, and keep them read-only. The server names the
model in every answer as `<name>@<first 12 hex digits of the ONNX file's sha256>`;
Colonyhall refuses to mix vectors of two models (`colonyhall rag-reembed`
switches over).

## Text embedding (Raspberry Pi): multilingual-e5-small, int8

- Repository: `Xenova/multilingual-e5-small`, commit `761b726dd34fb83930e26aab4e9ac3899aa1fa78`
- Files: `onnx/model_int8.onnx` (118 MB), `tokenizer.json`
- Server: `--mode embed --name multilingual-e5-small-int8`
- Answer: `multilingual-e5-small-int8@4d24e2bc01a4` (checked on the PC, 2026-11-10)

```sh
mkdir -p model/onnx && cd model
REV=761b726dd34fb83930e26aab4e9ac3899aa1fa78
curl -fL -o onnx/model_int8.onnx https://huggingface.co/Xenova/multilingual-e5-small/resolve/$REV/onnx/model_int8.onnx
curl -fL -o tokenizer.json       https://huggingface.co/Xenova/multilingual-e5-small/resolve/$REV/tokenizer.json
sha256sum onnx/model_int8.onnx   # the first 12 digits: 4d24e2bc01a4
chmod -R a-w .
```

## Text reranker (homelab): the starting choice, to be measured there

- `jinaai/jina-reranker-v2-base-multilingual`, commit `9cfeff2df7d40d1b78e75e5e9cebec92a99813c9`,
  `onnx/model_int8.onnx` + `tokenizer.json`; `--name jina-reranker-v2-int8` → `jina-reranker-v2-int8@c5220cf8fe02`.
  On the PC (Ryzen 5950X, 2 threads) about 0.6 s for ~20 candidates; the homelab's
  i5-2500S (AVX only) is slower: **measure it** (`docs/13`), and if 30 candidates
  take more than 2 s, set `COLONY_RERANK_TOP=20` or use the smaller one below.
- Smaller: `cross-encoder/mmarco-mMiniLMv2-L12-H384-v1`, commit `1427fd652930e4ba29e8149678df786c240d8825`,
  `onnx/model.onnx` + `tokenizer.json`.
