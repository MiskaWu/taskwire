const HDR = { 'X-Taskwire-UI': '1' };
export async function api(path, body) {
  try {
    const res = body
      ? await fetch(path, { method: 'POST', headers: { ...HDR, 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      : await fetch(path, { headers: HDR });
    return await res.json();
  } catch (e) {
    return { ok: false, error: '控制台連不上：' + e.message };
  }
}
// SSE 跟隨 log。EventSource 帶不了自訂標頭——後端的 GET 端點只驗 Host，設計上允許。
export function streamLog(name, onLine, onErr) {
  const es = new EventSource(`/api/log/stream?name=${encodeURIComponent(name)}`);
  es.onmessage = (e) => onLine(JSON.parse(e.data));
  es.onerror = () => onErr && onErr();
  return es;
}
