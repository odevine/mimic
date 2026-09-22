// Every fetch and server-sent-event subscription the frontend makes, so the
// HTTP surface is readable in one place

async function json(resp) {
  if (!resp.ok) throw new Error((await resp.text()).trim() || resp.statusText);
  return resp.json();
}

function get(path, opts) {
  return fetch(path, opts).then(json);
}

function send(method, path, body, opts) {
  return fetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    ...opts,
  }).then(json);
}

// watchJob follows a job's event stream, calling onStep for progress and
// resolving with the terminal event. It rejects when the job reports an error
// or the connection drops. The returned promise carries close() so a caller can
// abandon a job it has superseded
export function watchJob(path, onStep) {
  let es;
  const p = new Promise((resolve, reject) => {
    es = new EventSource(path);
    es.onmessage = (ev) => {
      const e = JSON.parse(ev.data);
      if (e.done) {
        es.close();
        if (e.error) reject(new Error(e.error));
        else resolve(e);
        return;
      }
      if (onStep) onStep(e.step, e.frac);
    };
    es.onerror = () => {
      es.close();
      reject(new Error("connection lost"));
    };
  });
  p.close = () => es && es.close();
  return p;
}

export const api = {
  capabilities: () => get("/api/capabilities"),
  settings: () => get("/api/settings"),
  saveSettings: (s) => send("PUT", "/api/settings", s),

  search: (q, signal) => get(`/api/search?q=${encodeURIComponent(q)}`, { signal }),
  recents: () => get("/api/recents"),
  printings: (name, signal) => get(`/api/printings?name=${encodeURIComponent(name)}`, { signal }),
  symbolURL: (code, px = 36) => `/api/symbol?code=${encodeURIComponent(code)}&px=${px}`,

  // render starts a job for one target, "preview" or "output", and resolves to
  // { jobId, dpi }
  render: (base, edits, target) => send("POST", "/api/render", { base, edits, target }),
  renderEvents: (jobId, onStep) => watchJob(`/api/render/${jobId}/events`, onStep),
  renderImageURL: (jobId, download) =>
    `/api/render/${jobId}/image${download ? "?download=1" : `?ts=${Date.now()}`}`,

  resolution: () => get("/api/resolution"),
  saveResolution: (previewDpi, outputDpi) => send("POST", "/api/resolution", { previewDpi, outputDpi }),

  templates: () => get("/api/templates"),
  activeTemplate: () => get("/api/template/active"),
  selectTemplate: (name, version) => send("POST", "/api/template/select", { name, version }),
  selectEvents: (jobId, onStep) => watchJob(`/api/template/select/${jobId}/events`, onStep),
};
