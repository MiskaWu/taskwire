import { useEffect, useState } from 'react';
import { api } from '../api';
import { Badge, Btn, Card, ErrBox, Tile, Spin } from '../ui';
import { Topbar } from '../App';
import type { ActionResult, IssuesResp, LogResp, PageProps, TokenResp } from '../types';

export default function Overview({ st }: PageProps) {
  const [issues, setIssues] = useState<IssuesResp | null>(null);
  const [token, setToken] = useState<TokenResp | null>(null);
  const [tail, setTail] = useState<LogResp | null>(null);
  const [doctor, setDoctor] = useState<ActionResult | null>(null);
  const [busy, setBusy] = useState('');
  const [flash, setFlash] = useState<{ key: string; r: ActionResult } | null>(null);

  useEffect(() => {
    api<never>('/api/issues').then((r) => setIssues(r as IssuesResp));
    api<never>('/api/tokeninfo').then((r) => setToken(r as TokenResp));
    api<never>('/api/log?name=dispatch.log&lines=14').then((r) => setTail(r as LogResp));
  }, []);

  const run = async (key: string, path: string, body?: unknown) => {
    setBusy(key);
    const r = await api<ActionResult & object>(path, body);
    setBusy('');
    if (key === 'doctor') setDoctor(r);
    else setFlash({ key, r });
    if (key === 'wake') setTimeout(() => api<never>('/api/log?name=dispatch.log&lines=14').then((res) => setTail(res as LogResp)), 1800);
  };

  const webhook = st?.ok ? st.units.find((u) => u.name === 'task-webhook.service') : undefined;
  const timers = st?.ok ? st.units.filter((u) => u.kind === 'timer') : [];
  const creds = st?.ok ? st.creds : null;
  const b = issues?.ok ? issues.buckets : null;
  const activeN = st?.ok ? st.units.filter((u) => u.active === 'active').length : null;

  return (<>
    <Topbar title="總覽" sub={`系統健康、球權與最近活動${issues?.ok ? ` — ${issues.fetched_at} 快照` : ''}`}>
      <Btn onClick={() => run('doctor', '/api/doctor')} disabled={busy === 'doctor'}>
        {busy === 'doctor' ? <Spin /> : '跑 task doctor'}</Btn>
      <Btn onClick={() => run('healthz', '/api/healthz')}>戳門鈴 healthz</Btn>
      <Btn kind="primary" onClick={() => run('wake', '/api/dispatch/run', {})}>手動叫醒 dispatch</Btn>
    </Topbar>
    <div className="content">
      {flash && (flash.r.ok
        ? <div className="okmsg">{flash.key === 'healthz' ? `門鈴回應：${flash.r.text}` : flash.r.text || '完成'}</div>
        : <ErrBox r={flash.r} />)}
      {doctor && (
        <Card title="task doctor" right={<Btn onClick={() => setDoctor(null)}>收起</Btn>}>
          <pre className="pane">{doctor.text}</pre>
          {!doctor.ok && <div className="note" style={{ marginTop: 8 }}>有一項以上不合格——內容在上面。</div>}
        </Card>
      )}
      <div className="grid g4">
        <Tile label="門鈴 WEBHOOK"
          badge={webhook ? <Badge k={webhook.active === 'active' ? 'ok' : (webhook.note ? 'warn' : 'bad')}>
            {webhook.enabled === 'generated' ? 'quadlet' : webhook.active}</Badge> : null}
          value={webhook ? (webhook.active === 'active' ? '運作中' : webhook.note ? '未設定' : '停著') : '…'}
          sub={webhook?.note ? '未設密鑰——不啟動是設計' : `:${st?.ok ? st.settings.find((s) => s.key === 'TASK_WEBHOOK_PORT')?.effective : ''} · 事件驗簽投信箱`} />
        <Tile label="無頭憑證"
          badge={creds && <Badge k={creds.ok ? 'ok' : 'bad'}>{creds.ok ? 'refresh 活著' : '異常'}</Badge>}
          value={creds ? creds.label : '…'} sub={creds?.hint || '48 小時內有成功刷新即為正常'} />
        <Tile label="GLAB TOKEN" mono
          badge={token && (token.ok
            ? <Badge k={(token.days ?? 999) < 30 ? 'warn' : 'ok'}>{token.expires_at}</Badge>
            : <Badge k="bad">查不到</Badge>)}
          value={token ? (token.ok ? `${token.days} 天` : '—') : '…'}
          sub={token?.ok ? '到期前 30 天日報會示警（#32）' : (token && !token.ok && token.error) || ''} />
        <Tile label="服務" mono value={st?.ok && activeN !== null ? `${activeN} / ${st.units.length}` : '…'}
          badge={st?.ok && <Badge k={activeN === st.units.length ? 'ok' : 'warn'}>
            {activeN === st.units.length ? '全部 active' : '有服務停著'}</Badge>}
          sub="webhook · 信箱監看 · 控制台 · 兩個 timer" />
      </div>
      <div className="grid g2">
        <Card title="球在誰手上" sub="拉 todo 與關單是你的動作——這裡只報數，不代勞">
          {issues === null ? <Spin /> : !issues.ok || !b ? <ErrBox r={issues} /> : (<>
            <div style={{ fontSize: 15, fontWeight: 600, marginBottom: 12 }}>
              {b.done.length + b.blocked.length === 0 ? '乾淨——沒有在等你的。' : '有球在你手上：'}
            </div>
            <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
              <div className={`stat ${b.done.length ? 'hi' : ''}`}><span className="n">{b.done.length}</span><span className="muted">做完待驗收</span></div>
              <div className={`stat ${b.blocked.length ? 'hi' : ''}`}><span className="n">{b.blocked.length}</span><span className="muted">卡住等回話</span></div>
              <div className="stat hi"><span className="n">{b.inbox.length}</span><span>收集箱待談定</span></div>
              <a href="#tickets" style={{ marginLeft: 'auto', fontSize: 12.5, fontWeight: 600 }}>看單況</a>
            </div>
          </>)}
        </Card>
        <Card title="排程" sub="改動寫 drop-in，repo 出廠值不動">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            {timers.map((t) => (
              <div key={t.name} style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={{ flex: 1 }}>{t.label}</span>
                <span className="mono" style={{ fontSize: 11.5, color: 'var(--ink2)' }}>{t.oncalendar || '—'}</span>
                <Badge k={t.active === 'active' ? 'idle' : 'warn'}>{t.active === 'active' ? '排程中' : t.active}</Badge>
              </div>
            ))}
          </div>
        </Card>
      </div>
      <Card title="最近活動" sub="dispatch.log 尾端——完整內容與即時跟隨在「紀錄」頁" style={{ flex: '1 1 auto' }}>
        {tail === null ? <Spin /> : !tail.ok ? <ErrBox r={tail} /> : (
          tail.text.trim() ? tail.text.trim().split('\n').map((ln, i) => (
            <div key={i} className="actline"><span>{ln}</span></div>
          )) : <div className="empty">（還沒有活動）</div>
        )}
      </Card>
    </div>
  </>);
}
