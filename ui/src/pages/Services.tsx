import { useState } from 'react';
import { api } from '../api';
import { Badge, Btn, Card, ErrBox, Spin } from '../ui';
import { Topbar } from '../App';
import type { ActionResult, PageProps, TextResp, UnitInfo } from '../types';

type JournalState = TextResp | { loading: true } | null;

function Unit({ u, reload }: { u: UnitInfo; reload: () => void }) {
  const [msg, setMsg] = useState<ActionResult | null>(null);
  const [journal, setJournal] = useState<JournalState>(null);
  const [sched, setSched] = useState(u.oncalendar || '');
  const [busy, setBusy] = useState(false);

  const act = async (action: string) => {
    setBusy(true); setMsg(null);
    const r = await api<object>('/api/service', { unit: u.name, action });
    setBusy(false);
    setMsg(r.ok ? { ok: true, text: `${action} 完成` } : r);
    if (r.ok) setTimeout(reload, 700);
  };
  const applySched = async () => {
    setBusy(true); setMsg(null);
    const r = await api<{ parsed?: string }>('/api/schedule', { unit: u.name, oncalendar: sched });
    setBusy(false);
    setMsg(r.ok ? { ok: true, text: `已套用${r.parsed ? '：' + (r.parsed.split('\n').find((l) => l.includes('Next')) || '') : ''}` } : r);
    if (r.ok) setTimeout(reload, 700);
  };
  const resetSched = async () => {
    const r = await api<object>('/api/schedule/reset', { unit: u.name });
    setMsg(r.ok ? { ok: true, text: '已還原成 repo 出廠排程' } : r);
    if (r.ok) setTimeout(reload, 700);
  };
  const showJournal = async () => {
    setJournal({ loading: true });
    setJournal(await api<{ text: string }>(`/api/journal?unit=${encodeURIComponent(u.name)}&lines=60`));
  };

  const stateBadge = u.active === 'unknown' ? <Badge k="warn">狀態不明</Badge>
    : u.active === 'active' ? <Badge k="ok">{u.sub === 'running' ? '運行中' : u.kind === 'timer' ? '排程中' : u.kind === 'path' ? '監看中' : '已啟用'}</Badge>
    : u.active === 'failed' ? <Badge k={u.note ? 'warn' : 'bad'}>{u.note ? '未設定' : '失敗'}</Badge>
    : <Badge k="idle">{u.active}</Badge>;
  const enBadge = u.enabled === 'enabled' ? <Badge k="accent">開機自啟</Badge>
    : u.enabled === 'generated' ? <Badge k="accent">quadlet 管理</Badge> : null;

  return (<>
    <div className="srow">
      <div><div className="nm">{u.label}</div><div className="un">{u.name}</div></div>
      <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>{stateBadge}{enBadge}</div>
      <div className="acts">
        {u.active === 'active'
          ? <><Btn onClick={() => act('restart')} disabled={busy}>重啟</Btn>
              <Btn kind="danger" onClick={() => act('stop')} disabled={busy}>停止</Btn></>
          : <Btn kind="primary" onClick={() => act('start')} disabled={busy}>啟動</Btn>}
        {u.enabled !== 'generated' && (u.enabled === 'enabled'
          ? <Btn kind="danger" onClick={() => act('disable')} disabled={busy}>取消自啟</Btn>
          : <Btn onClick={() => act('enable')} disabled={busy}>設為自啟</Btn>)}
        <Btn onClick={showJournal}>journal</Btn>
      </div>
    </div>
    {(u.help || u.note) && (
      <div style={{ padding: '6px 0 10px', borderBottom: '1px solid var(--line2)' }}>
        {u.help && <div className="muted" style={{ fontSize: 12 }}>{u.help}</div>}
        {u.note && <div className="note" style={{ marginTop: 6 }}>{u.note}</div>}
      </div>
    )}
    {u.kind === 'timer' && (
      <div style={{ display: 'flex', gap: 8, alignItems: 'center', padding: '10px 0', borderBottom: '1px solid var(--line2)' }}>
        <input value={sched} onChange={(e) => setSched(e.target.value)}
          placeholder="例：hourly、daily、*-*-* 09:00" style={{ flex: 1 }} />
        <Btn onClick={applySched} disabled={busy}>套用</Btn>
        <Btn onClick={resetSched} disabled={busy}>還原</Btn>
        {u.next && u.next !== '0' && <span className="mono" style={{ fontSize: 11.5, color: 'var(--ink2)' }}>下次 {u.next}</span>}
      </div>
    )}
    {msg && (msg.ok ? <div className="okmsg" style={{ margin: '8px 0' }}>{msg.text}</div> : <ErrBox r={msg} />)}
    {journal && ('loading' in journal ? <Spin /> : journal.ok
      ? <pre className="pane" style={{ margin: '8px 0' }}>{journal.text}</pre>
      : <ErrBox r={journal} />)}
  </>);
}

export default function Services({ st, reload }: PageProps) {
  if (!st) return <><Topbar title="服務與排程" sub="載入中…" /><div className="content"><Spin /></div></>;
  if (!st.ok) return <><Topbar title="服務與排程" sub="狀態讀取失敗" /><div className="content"><ErrBox r={st} /></div></>;
  const services = st.units.filter((u) => u.kind !== 'timer');
  const timers = st.units.filter((u) => u.kind === 'timer');
  return (<>
    <Topbar title="服務與排程" sub="開關、重啟、看 journal；排程改動寫 drop-in——之後重裝也不會被洗掉" />
    <div className="content">
      {!st.systemd && <div className="note">碰不到 user systemd——動作會降級成「給你指令」，按下去不會直接執行，而是把該跑的那行顯示出來讓你複製。</div>}
      <Card title="常駐服務">{services.map((u) => <Unit key={u.name} u={u} reload={reload} />)}</Card>
      <Card title="排程（timer）" sub="格式即 systemd OnCalendar，例：hourly、daily、*-*-* 09:00、Mon..Fri 08:30">
        {timers.map((u) => <Unit key={u.name} u={u} reload={reload} />)}
        <div className="muted" style={{ paddingTop: 10, fontSize: 12 }}>
          手動叫醒 dispatch 在「總覽」——那是按門鈴，不是指派工作：dispatch 醒來自己用 task next 重讀真實狀態。
        </div>
      </Card>
    </div>
  </>);
}
