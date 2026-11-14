// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { ApiError, getCsrfToken, type Problem, type Schemas } from "./client";

/**
 * Uploads a file to a channel with progress (fetch cannot report upload
 * progress). Returns the stored file; rejects with ApiError, or with an
 * AbortError when signal aborts. The server's limit answers 413.
 */
export function uploadFile(
  channelId: string,
  file: File,
  onProgress: (fraction: number) => void,
  signal?: AbortSignal,
): Promise<Schemas["File"]> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `/api/v1/channels/${encodeURIComponent(channelId)}/files`);
    xhr.withCredentials = true;
    const csrf = getCsrfToken();
    if (csrf) xhr.setRequestHeader("X-CSRF-Token", csrf);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onload = () => {
      let body: unknown;
      try {
        body = JSON.parse(xhr.responseText);
      } catch {
        body = undefined;
      }
      if (xhr.status === 201) resolve(body as Schemas["File"]);
      else reject(new ApiError(xhr.status, body as Problem | undefined));
    };
    xhr.onerror = () => reject(new ApiError(0, undefined));
    xhr.onabort = () => reject(new DOMException("aborted", "AbortError"));
    signal?.addEventListener("abort", () => xhr.abort());
    const form = new FormData();
    form.append("file", file);
    xhr.send(form);
  });
}
