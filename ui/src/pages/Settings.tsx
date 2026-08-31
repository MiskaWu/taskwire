import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { api } from '../api';
import { Badge, Btn, Card, ErrBox, Spin } from '../ui';
import { Topbar } from '../App';
import type { ActionResult, Flash, PageProps, SecretValueResp, SkillResp } from '../types';

const strong = (s: string): ReactNode =>
  s.split('**').map((seg, i) => i % 2 ? <strong key={i}>{seg}</strong> : seg);

function SettingsForm({ st, reload }: PageProps) {
  const [vals, setVals] = useState<Record<string, string> | null>(null);
  const [msg, setMsg] = useState<Flash | null>(null);
  if (st?.ok && vals === null) setVals(Object.fromEntries(st.settings.map((s) => [s.key, s.value])));
  if (!st?.ok || vals === null) return <Spin />;

  const save = async () => {
    setMsg({ busy: true });
    const r = await api<object>('/api/config', { values: vals });
    setMsg(r);
    if (r.ok) reload();
  };
  const srcBadge: Record<string, [string, string]> = {
    env: ['warn', 'env 蓋住'], file: ['accent', '設定檔'], default: ['idle', '內建預設'],
  };

  return (<>
    {st.settings.map((s) => {
      const [k, label] = srcBadge[s.source];
      return (
        <div key={s.key} className="frow">
          <div className="head">
            <span style={{ fontWeight: 600 }}>{s.label}</span>
            <span className="k">{s.key}</span>
            <Badge k={k}>{label}</Badge>
          </div>
          <input value={vals[s.key]} placeholder={s.default}
            onChange={(e) => setVals({ ...vals, [s.key]: e.target.value })} />
          <div className="help">{strong(s.help)}</div>
        </div>
      );
    })}
    <div style={{ display: 'flex', gap: 8, alignItems: 'center', paddingTop: 12, flexWrap: 'wrap' }}>
      <Btn kind="primary" onClick={save}>儲存設定</Btn>
      {msg && 'busy' in msg && <Spin />}
      {msg && !('busy' in msg) && (msg.ok ? (
        <div className="okmsg" style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          已寫入設定檔
          {(msg.shadowed?.length ?? 0) > 0 && <span>⚠ {msg.shadowed!.join('、')} 被環境變數蓋住，改了不生效</span>}
          {msg.restart_needed?.map((u) => (
            <Btn key={u} onClick={async () => { await api('/api/service', { unit: u, action: 'restart' }); reload(); }}>
              重啟 {u}</Btn>
          ))}
        </div>
      ) : <ErrBox r={msg} />)}
    </div>
  </>);
}

function Secrets({ st, reload }: PageProps) {
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [loaded, setLoaded] = useState<Record<string, boolean>>({});
  const [msg, setMsg] = useState<Record<string, ActionResult>>({});
  if (!st?.ok) return <Spin />;

  const put = (name: string, r: ActionResult) => setMsg((m) => ({ ...m, [name]: r }));
  const reveal = async (name: string) => {
    const r = await api<never>(`/api/secret?name=${encodeURIComponent(name)}`) as SecretValueResp;
    if (!r.ok) return put(name, r);
    setInputs((v) => ({ ...v, [name]: r.value }));
    setLoaded((v) => ({ ...v, [name]: true }));
  };
  const save = async (name: string) => {
    const val = inputs[name] ?? '';
    if (!loaded[name] && val.includes('…')) {
      return put(name, { ok: false, error: '框裡是遮蔽後的摘要，直接存會把密鑰寫壞。先按「顯示」再改。' });
    }
    const r = await api<{ restart_needed?: string[] }>('/api/secret', { name, value: val });
    put(name, r.ok ? { ok: true, text: '已寫入（0600）' + (r.restart_needed?.length ? '——門鈴要重啟才讀得到新密鑰' : '') } : r);
    if (r.ok) reload();
  };
  const regen = async (name: string) => {
    if (!confirm('重新產生之後，GitLab Webhook 設定裡的 Signing token 也要換成新值，否則門鈴會開始 403。要繼續嗎？')) return;
    const r = await api<{ value: string }>('/api/secret/generate', { name });
    if (!r.ok) return put(name, r);
    setInputs((v) => ({ ...v, [name]: r.value }));
    setLoaded((v) => ({ ...v, [name]: true }));
    put(name, { ok: true, text: '新密鑰已寫入。去 GitLab 換 Signing token，然後重啟門鈴。' });
    reload();
  };

  return (<>
    {st.secrets.map((s) => (
      <div key={s.name} className="frow">
        <div className="head">
          <span style={{ fontWeight: 600 }}>{s.label}</span>
          <span className="k">{s.name}</span>
          <Badge k={s.present ? 'ok' : 'bad'}>{s.present ? `已設定 · ${s.length} 字` : '未設定'}</Badge>
          {s.mode && s.mode !== '0o600' && <Badge k="warn">權限 {s.mode}，應為 0600</Badge>}
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <input style={{ flex: 1 }} placeholder="（未設定）"
            value={inputs[s.name] ?? s.hint} onChange={(e) => {
              setInputs((v) => ({ ...v, [s.name]: e.target.value }));
              setLoaded((v) => ({ ...v, [s.name]: true }));
            }} />
          <Btn onClick={() => reveal(s.name)}>顯示</Btn>
          <Btn onClick={() => save(s.name)}>儲存</Btn>
          {s.generate === 'hex32' && <Btn kind="danger" onClick={() => regen(s.name)}>重新產生</Btn>}
        </div>
        <div className="help">{strong(s.help)}</div>
        {msg[s.name] && (msg[s.name].ok
          ? <div className="okmsg">{msg[s.name].text}</div> : <ErrBox r={msg[s.name]} />)}
      </div>
    ))}
    <div className="note" style={{ marginTop: 10 }}>重產密鑰後：GitLab 的 Signing token 要換成新值，門鈴要重啟才讀得到。</div>
  </>);
}

function Notify({ st, reload }: PageProps) {
  const [kind, setKind] = useState('測試');
  const [text, setText] = useState('控制台測試訊息');
  const [out, setOut] = useState<Flash | null>(null);
  const threads = st?.ok ? st.threads : {};

  const send = async () => {
    setOut({ busy: true });
    setOut(await api<object>('/api/notify-test', { kind, message: text }));
    reload();
  };
  const drop = async (key: string) => { await api('/api/threads/reset', { key }); reload(); };

  return (<>
    <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
      <input value={kind} onChange={(e) => setKind(e.target.value)} style={{ width: 90 }} />
      <input value={text} onChange={(e) => setText(e.target.value)} style={{ flex: 1 }} />
      <Btn onClick={send}>發一則</Btn>
    </div>
    {out && ('busy' in out ? <Spin /> : out.ok
      ? <div className="okmsg"><pre style={{ margin: 0, fontFamily: 'var(--mono)', fontSize: 11.5, whiteSpace: 'pre-wrap' }}>{out.text}</pre></div>
      : <ErrBox r={out} />)}
    <div style={{ fontSize: 12, fontWeight: 600, margin: '12px 0 4px' }}>
      討論串對照表 <span className="muted" style={{ fontWeight: 400 }}>——刪掉某列，下次該類型自動開新串</span>
    </div>
    {Object.keys(threads).length === 0
      ? <div className="empty">（還沒有任何串——第一則該類型的通知會自動開一條）</div>
      : Object.entries(threads).map(([k, v]) => (
        <div key={k} style={{ display: 'grid', gridTemplateColumns: '80px 1fr 72px', gap: 10, alignItems: 'center', padding: '6px 0', borderBottom: '1px solid var(--line2)' }}>
          <span>{k}</span>
          <span className="mono" style={{ fontSize: 11.5, color: 'var(--ink2)' }}>{String(v)}</span>
          <Btn onClick={() => drop(k)}>刪掉</Btn>
        </div>
      ))}
  </>);
}

function SkillHints() {
  const [body, setBody] = useState<string | { ok: false; error?: string } | null>(null);
  const [msg, setMsg] = useState<ActionResult | null>(null);
  useEffect(() => {
    api<never>('/api/skill').then((res) => {
      const r = res as SkillResp;
      setBody(r.ok ? r.body : r);
    });
  }, []);
  if (body === null) return <Spin />;
  if (typeof body === 'object') return <ErrBox r={body} />;
  const save = async () => {
    setMsg(await api<object>('/api/skill', { body }));
    setTimeout(() => setMsg(null), 5000);
  };
  return (<>
    <textarea value={body} onChange={(e) => setBody(e.target.value)} spellCheck={false} />
    <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 8 }}>
      <Btn kind="primary" onClick={save}>儲存提示</Btn>
      {msg && (msg.ok ? <div className="okmsg">已寫回 SKILL.md（symlink 改即生效）</div> : <ErrBox r={msg} />)}
    </div>
  </>);
}

export default function Settings({ st, reload }: PageProps) {
  return (<>
    <Topbar title="設定與密鑰"
      sub={`寫進 ${st?.ok ? st.config_path : '…'}——留空回內建預設；被環境變數蓋住的欄位會標出來`} />
    <div className="content">
      <div className="grid gset">
        <Card title="設定"><SettingsForm st={st} reload={reload} /></Card>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <Card title="密鑰" sub="0600 獨立檔，預設遮蔽"><Secrets st={st} reload={reload} /></Card>
          <Card title="通知" sub="測試只推 Discord（-d），不動 GitLab 健康卡"><Notify st={st} reload={reload} /></Card>
        </div>
      </div>
      <Card title="判斷提示（SKILL.md）"
        sub="拍板一號：模型判斷不足時在這裡加一條帶日期與理由的提示，不是加機械閘門。上限五條，加第六條先刪一條。">
        <SkillHints />
      </Card>
    </div>
  </>);
}
