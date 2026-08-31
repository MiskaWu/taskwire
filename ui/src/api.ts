import type { Maybe } from './types';

const HDR = { 'X-Taskwire-UI': '1' };

export async function api<T>(path: string, body?: unknown): Promise<Maybe<T>> {
  try {
    const res = body !== undefined
      ? await fetch(path, {
          method: 'POST',
          headers: { ...HDR, 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        })
      : await fetch(path, { headers: HDR });
    return (await res.json()) as Maybe<T>;
  } catch (e) {
    return { ok: false, error: '控制台連不上：' + (e instanceof Error ? e.message : String(e)) };
  }
}

// SSE 跟隨 log。EventSource 帶不了自訂標頭——後端的 GET 端點只驗 Host，設計上允許。
export function streamLog(name: string, onLine: (line: string) => void, onErr?: () => void): EventSource {
  const es = new EventSource(`/api/log/stream?name=${encodeURIComponent(name)}`);
  es.onmessage = (e) => onLine(JSON.parse(e.data) as string);
  es.onerror = () => onErr && onErr();
  return es;
}
