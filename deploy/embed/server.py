#!/usr/bin/env python3
"""Colonyhall's small model server for the search (round 36,
docs/11-rag-terv.md): text embeddings on the Raspberry Pi
(POST /v1/embeddings, OpenAI-compatible) or reranking on the homelab
(POST /v1/rerank, Jina/Cohere-style), with ONNX Runtime.

One request is computed at a time (the Pi has four slow cores and 1 GB);
others wait. The model name in every answer carries the first 12 hex
digits of the ONNX file's sha256, so Colonyhall notices a model change.
The server is dumb on purpose: Colonyhall adds the e5 "query: " and
"passage: " prefixes, batches, and decides what to do on errors.

    python3 server.py --mode embed  --model /opt/colony-embed/model --port 8090
    python3 server.py --mode rerank --model /model --port 8091
"""

import argparse
import hashlib
import json
import os
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

# No usage telemetry: Colonyhall promises none (docs/17, 0.8). ONNX Runtime's
# Linux build carries a Microsoft 1DS client (mobile.events.data.microsoft.com)
# that starts while the module loads, so the switch must be set before the
# import below; disable_telemetry_events() in main() alone comes too late.
os.environ["ORT_DISABLE_TELEMETRY"] = "1"

import numpy as np  # noqa: E402
import onnxruntime as ort  # noqa: E402
from tokenizers import Tokenizer  # noqa: E402

MAX_BODY = 1 << 20  # 1 MB
MAX_INPUTS = 64  # embeddings per request
MAX_DOCS = 100  # rerank documents per request


def find_onnx(model_dir):
    """The ONNX file: the int8 one if there is one, else model.onnx, else
    the first *.onnx under onnx/."""
    for cand in ("model_int8.onnx", "model_quantized.onnx", "model.onnx"):
        for sub in ("", "onnx"):
            p = os.path.join(model_dir, sub, cand)
            if os.path.isfile(p):
                return p
    onnx_dir = os.path.join(model_dir, "onnx")
    if os.path.isdir(onnx_dir):
        for name in sorted(os.listdir(onnx_dir)):
            if name.endswith(".onnx"):
                return os.path.join(onnx_dir, name)
    raise SystemExit(f"no ONNX model in {model_dir}")


def sha12(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()[:12]


class Model:
    def __init__(self, mode, model_dir, name, threads, max_tokens, batch):
        self.mode = mode
        self.batch = batch
        path = find_onnx(model_dir)
        self.name = f"{name or os.path.basename(os.path.normpath(model_dir))}@{sha12(path)}"
        opts = ort.SessionOptions()
        opts.intra_op_num_threads = threads
        opts.inter_op_num_threads = 1
        # Without the arena the memory goes back after a big request (the Pi).
        opts.enable_cpu_mem_arena = False
        self.session = ort.InferenceSession(path, sess_options=opts, providers=["CPUExecutionProvider"])
        self.inputs = {i.name for i in self.session.get_inputs()}
        self.tok = Tokenizer.from_file(os.path.join(model_dir, "tokenizer.json"))
        self.tok.enable_truncation(max_length=max_tokens)
        self.tok.enable_padding()
        self.lock = threading.Lock()

    def _feed(self, encs):
        feed = {
            "input_ids": np.array([e.ids for e in encs], dtype=np.int64),
            "attention_mask": np.array([e.attention_mask for e in encs], dtype=np.int64),
        }
        if "token_type_ids" in self.inputs:
            feed["token_type_ids"] = np.array([e.type_ids for e in encs], dtype=np.int64)
        return {k: v for k, v in feed.items() if k in self.inputs}

    def embed(self, texts):
        out = []
        with self.lock:
            for i in range(0, len(texts), self.batch):
                encs = self.tok.encode_batch(texts[i : i + self.batch])
                feed = self._feed(encs)
                hidden = self.session.run(None, feed)[0]  # [batch, tokens, dims]
                mask = feed["attention_mask"][..., None].astype(np.float32)
                pooled = (hidden * mask).sum(axis=1) / np.clip(mask.sum(axis=1), 1e-9, None)
                pooled /= np.clip(np.linalg.norm(pooled, axis=1, keepdims=True), 1e-12, None)
                out.extend(pooled.astype(np.float32).tolist())
        return out

    def rerank(self, query, docs):
        scores = []
        with self.lock:
            for i in range(0, len(docs), self.batch):
                encs = self.tok.encode_batch([(query, d) for d in docs[i : i + self.batch]])
                logits = self.session.run(None, self._feed(encs))[0]
                logits = np.asarray(logits).reshape(len(encs), -1)[:, -1]
                scores.extend((1 / (1 + np.exp(-logits))).astype(float).tolist())
        return scores


def make_handler(model):
    class Handler(BaseHTTPRequestHandler):
        server_version = "colony-embed/1"

        def log_message(self, fmt, *args):  # one short line, no bodies
            sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))

        def send_json(self, status, obj):
            body = json.dumps(obj).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            if self.path == "/healthz":
                self.send_json(200, {"ok": True, "model": model.name, "mode": model.mode})
            elif self.path == "/v1/models":
                self.send_json(200, {"object": "list", "data": [{"id": model.name, "object": "model"}]})
            else:
                self.send_json(404, {"error": "not found"})

        def read_json(self):
            n = int(self.headers.get("Content-Length") or 0)
            if n <= 0 or n > MAX_BODY:
                raise ValueError("body missing or too large")
            return json.loads(self.rfile.read(n))

        def do_POST(self):
            try:
                req = self.read_json()
                if self.path == "/v1/embeddings" and model.mode == "embed":
                    texts = req.get("input")
                    if isinstance(texts, str):
                        texts = [texts]
                    if not isinstance(texts, list) or not texts or len(texts) > MAX_INPUTS or not all(isinstance(t, str) for t in texts):
                        raise ValueError(f"input: 1-{MAX_INPUTS} strings")
                    vecs = model.embed(texts)
                    self.send_json(200, {"object": "list", "model": model.name,
                                         "data": [{"object": "embedding", "index": i, "embedding": v} for i, v in enumerate(vecs)]})
                elif self.path == "/v1/rerank" and model.mode == "rerank":
                    query, docs = req.get("query"), req.get("documents")
                    if not isinstance(query, str) or not isinstance(docs, list) or not docs or len(docs) > MAX_DOCS or not all(isinstance(d, str) for d in docs):
                        raise ValueError(f"query and 1-{MAX_DOCS} documents")
                    scores = model.rerank(query, docs)
                    self.send_json(200, {"model": model.name,
                                         "results": [{"index": i, "relevance_score": s} for i, s in enumerate(scores)]})
                else:
                    self.send_json(404, {"error": "not found"})
            except (ValueError, json.JSONDecodeError) as e:
                self.send_json(400, {"error": str(e)})
            except Exception as e:  # the model failed: say so, keep serving
                sys.stderr.write(f"error: {e!r}\n")
                self.send_json(500, {"error": "inference failed"})

    return Handler


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--mode", choices=["embed", "rerank"], required=True)
    ap.add_argument("--model", required=True, help="folder with the ONNX file and tokenizer.json")
    ap.add_argument("--name", default="", help="model name in answers (default: the folder name)")
    ap.add_argument("--host", default="127.0.0.1")
    ap.add_argument("--port", type=int, default=8090)
    ap.add_argument("--threads", type=int, default=4)
    ap.add_argument("--max-tokens", type=int, default=512)
    ap.add_argument("--batch", type=int, default=8, help="inputs computed together (memory)")
    args = ap.parse_args()
    # Belt and braces next to ORT_DISABLE_TELEMETRY (top of the file).
    if hasattr(ort, "disable_telemetry_events"):
        ort.disable_telemetry_events()
    model = Model(args.mode, args.model, args.name, args.threads, args.max_tokens, args.batch)
    srv = ThreadingHTTPServer((args.host, args.port), make_handler(model))
    sys.stderr.write(f"colony-embed: {args.mode} {model.name} on {args.host}:{args.port}\n")
    srv.serve_forever()


if __name__ == "__main__":
    main()
