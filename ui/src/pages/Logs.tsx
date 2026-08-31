import { useEffect, useRef, useState } from 'react';
import { api, streamLog } from '../api';
import { Badge, Btn, ErrBox, Spin } from '../ui';
import { Topbar } from '../App';
import type { LogResp, PageProps } from '../types';

export default function Logs({ st }: PageProps) {
  const [name, setName] = useState('dispatch.log');
  const [r, setR] = useState<LogResp | null>(null);
  const [lines, setLines] = useState<string[]>([]);
  const [follow, setFollow] = useState(true);
  const [live, setLive] = useState(false);
  const esRef = useRef<EventSource | null>(null);
  const paneRef = useRef<HTMLDivElement | null>(null);

  // 換檔或切換跟隨：先抓 400 行存量，跟隨開著就再開 SSE 接增量。
  useEffect(() => {
    let dead = false;
    setR(null); setLines([]); setLive(false);
    api<never>(`/api/log?name=${encodeURIComponent(name)}&lines=400`).then((res) => {
      if (dead) return;
      const lr = res as LogResp;
      setR(lr);
      if (lr.ok) setLines(lr.text ? lr.text.replace(/\n$/, '').split('\n') : []);
    });
    if (follow) {
      const es = streamLog(name, (ln) => {
        setLive(true);
        setLines((prev) => [...prev.slice(-1500), ln]);
      }, () => setLive(false));
      esRef.current = es;
      return () => { dead = true; es.close(); esRef.current = null; };
    }
    return () => { dead = true; };
  }, [name, follow]);

  useEffect(() => {
    if (follow && paneRef.current) paneRef.current.scrollTop = paneRef.current.scrollHeight;
  }, [lines, follow]);

  const files = ['dispatch.log', ...(st?.ok ? st.run_logs.filter((n) => n !== 'dispatch.log') : [])];
  const tag = (n: string) => n === 'dispatch.log' ? '總帳' : ('#' + (n.match(/-([0-9]+)\.log$/)?.[1] || '?'));

  return (<>
    <Topbar title="紀錄" sub="無頭側發生的每件事——dispatch 總帳與每次取件的個別紀錄">
      {follow && <Badge k={live ? 'ok' : 'idle'}>{live ? '跟隨中' : '等待資料'}</Badge>}
      <Btn onClick={() => setFollow(!follow)}>{follow ? '暫停跟隨' : '開始跟隨'}</Btn>
    </Topbar>
    <div className="content" style={{ minHeight: 0 }}>
      <div className="logwrap">
        <div className="loglist">
          {files.map((n) => (
            <button key={n} className={`logfile ${n === name ? 'on' : ''}`} onClick={() => setName(n)}>
              <span className="n">{n}</span><span className="t">{tag(n)}</span>
            </button>
          ))}
        </div>
        <div className="logcol">
          {r === null ? <div className="logpane"><Spin /></div>
            : !r.ok ? <ErrBox r={r} />
            : <div className="logpane" ref={paneRef}>{lines.length ? lines.join('\n') : '（空的）'}</div>}
          <div className="muted" style={{ fontSize: 12 }}>
            {r?.ok && r.truncated ? `起點是最後 400 行（全檔 ${r.total_lines} 行）· ` : ''}
            {follow ? '新內容即時追加 · ' : ''}unit 的 journal 在「服務與排程」各列的 journal 鈕
          </div>
        </div>
      </div>
    </div>
  </>);
}
