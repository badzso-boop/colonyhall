# The search's model server (deploy/embed/server.py) as an image: the
# homelab reranker runs it (docs/13-rag-reranker-beallitasa.md). The model
# is mounted read-only at /model, never baked in.
FROM python:3.12-slim
RUN useradd --system --uid 10001 --no-create-home embed
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY server.py .
# ONNX Runtime's own usage telemetry stays off (server.py sets it too).
ENV ORT_DISABLE_TELEMETRY=1
USER embed
EXPOSE 8091
ENTRYPOINT ["python", "-I", "/app/server.py"]
