import { useEffect, useState } from 'react';
import { api } from '../api.js';
import { Badge, Btn, Card, ErrBox, Spin, days } from '../ui.jsx';
import { Topbar } from '../App.jsx';

const GROUPS = [
  ['doing', '動手中 doing', 'accent'],
  ['blocked', '卡住等回話 blocked', 'bad'],
  ['done', '做完待驗收 done', 'ok'],
  ['todo', '排隊中 todo', 'idle'],
  ['inbox', '收集箱（還沒談定要做）', 'idle'],
];

export default function Tickets() {
  const [r, setR] = useState(null);
  const load = async (force) => {
    setR(null);
    setR(await api('/api/issues' + (force ? '?force=1' : '')));
  };
  useEffect(() => { load(false); }, []);

  return (<>
    <Topbar title="單況"
      sub={r?.ok ? `${r.repo} · ${r.total} 張 open · ${r.fetched_at} 讀取` : '讀取中…'}>
      <Badge k="idle">唯讀</Badge>
      <Btn onClick={() => load(true)}>重新查詢</Btn>
    </Topbar>
    <div className="content">
      <Card style={{ flex: '1 1 auto' }}>
        <div className="muted" style={{ paddingBottom: 10, borderBottom: '1px solid var(--line2)', fontSize: 12.5 }}>
          拉 todo（授權）與關單（驗收）是你的動作，這一頁刻意沒有那兩顆按鈕——理由跟{' '}
          <span className="mono">bin/task</span> 裡沒有那兩個指令是同一個。要動狀態，點編號進 GitLab。
        </div>
        {r === null ? <div style={{ padding: 16 }}><Spin /> 查詢中…</div>
          : !r.ok ? <div style={{ marginTop: 10 }}><ErrBox r={r} /></div>
          : GROUPS.map(([key, label, k]) => {
            const rows = r.buckets[key] || [];
            return (
              <div key={key}>
                <div className="ghead">{label}<Badge k={rows.length ? k : 'idle'}>{rows.length}</Badge></div>
                {rows.length === 0 ? <div className="empty">（沒有）</div>
                  : rows.map((x) => (
                    <div key={x.iid} className="trow">
                      <a className="mono" style={{ fontSize: 12.5 }} href={x.url} target="_blank" rel="noopener noreferrer">#{x.iid}</a>
                      <span>{x.title}</span>
                      <span className="days">{days(x.updated_at)} 天</span>
                    </div>
                  ))}
              </div>
            );
          })}
      </Card>
    </div>
  </>);
}
