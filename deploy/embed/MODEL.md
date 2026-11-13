# Models for the search (round 36)

The model files are not in the repository. Download them on the machine that
serves them, check the sha256, and keep them read-only. The server names the
model in every answer as `<name>@<first 12 hex digits of the ONNX file's sha256>`;
Colonyhall refuses to mix vectors of two models (`colonyhall rag-reembed`
switches over).

## Text embedding (a container next to the server, or a Raspberry Pi): multilingual-e5-small, int8

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

## Text reranker (homelab): mmarco-mMiniLMv2 (measured 2026-11-13)

- **In use:** `cross-encoder/mmarco-mMiniLMv2-L12-H384-v1`, commit `1427fd652930e4ba29e8149678df786c240d8825`,
  `onnx/model.onnx` (fp32, 471 MB) + `tokenizer.json`, licence Apache-2.0;
  `--name mmarco-mminilmv2-l12` → `mmarco-mminilmv2-l12@3e9a03ed1e96`.
  On the homelab (i5-2500S, AVX only, 2 threads, `--batch 1`): 20 real chunks in about 2.9 s; about 825 MB memory.

```sh
mkdir -p model/onnx && cd model
REV=1427fd652930e4ba29e8149678df786c240d8825
curl -fL -o onnx/model.onnx https://huggingface.co/cross-encoder/mmarco-mMiniLMv2-L12-H384-v1/resolve/$REV/onnx/model.onnx
curl -fL -o tokenizer.json  https://huggingface.co/cross-encoder/mmarco-mMiniLMv2-L12-H384-v1/resolve/$REV/tokenizer.json
sha256sum onnx/model.onnx   # the first 12 digits: 3e9a03ed1e96
chmod -R a-w .
```

- **Not used:** `jinaai/jina-reranker-v2-base-multilingual`, commit `9cfeff2df7d40d1b78e75e5e9cebec92a99813c9`,
  `onnx/model_int8.onnx` → `jina-reranker-v2-int8@c5220cf8fe02`. About 0.6 s for ~20 candidates on a
  Ryzen 5950X, but **17 s on the homelab's AVX-only CPU** (the fast int8 kernels need AVX2 or VNNI), and its
  licence is **CC BY-NC 4.0** (non-commercial). Consider it only on a newer CPU or a GPU, and only for
  non-commercial use.
