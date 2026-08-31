import { useEffect, useState, useCallback } from 'react';
import { api } from './api.js';
import { Icon, Dot } from './ui.jsx';
import Overview from './pages/Overview.jsx';
import Services from './pages/Services.jsx';
import Tickets from './pages/Tickets.jsx';
import Logs from './pages/Logs.jsx';
import Settings from './pages/Settings.jsx';

const NAV = [
  ['overview', '總覽', 'gauge'],
  ['services', '服務與排程', 'server'],
  ['tickets', '單況', 'board'],
  ['logs', '紀錄', 'file'],
  ['settings', '設定與密鑰', 'sliders'],
];

export default function App() {
  const [route, setRoute] = useState(location.hash.slice(1) || 'overview');
  const [st, setSt] = useState(null);

  const reload = useCallback(async () => setSt(await api('/api/state')), []);
  useEffect(() => {
    reload();
    const t = setInterval(reload, 15000); // 狀態每 15 秒背景刷新
    const onHash = () => setRoute(location.hash.slice(1) || 'overview');
    addEventListener('hashchange', onHash);
    return () => { clearInterval(t); removeEventListener('hashchange', onHash); };
  }, [reload]);

  const allUp = st?.ok && st.units.every((u) => u.active === 'active');
  const repo = st?.ok ? (st.settings.find((s) => s.key === 'TASK_REPO')?.effective || '') : '';
  const Page = { overview: Overview, services: Services, tickets: Tickets, logs: Logs, settings: Settings }[route] || Overview;

  return (
    <div className="shell">
      <div className="side">
        <div className="brand">
          <Dot c={allUp ? '#5ed99a' : '#f0c060'} />
          <div>
            <div className="name">taskwire</div>
            <div className="sub">控制台</div>
          </div>
        </div>
        <nav className="nav">
          {NAV.map(([key, label, ic]) => (
            <button key={key} className={`nav-item ${route === key ? 'on' : ''}`}
              onClick={() => { location.hash = key; }}>
              <Icon name={ic} />{label}
            </button>
          ))}
        </nav>
        <div className="side-foot">
          <div className="row">
            <Dot c={st ? (st.systemd ? '#5ed99a' : '#f0c060') : '#6b7484'} />
            {st ? (st.systemd ? 'systemd 可用' : 'systemd 降級模式') : '載入中…'}
          </div>
          <div className="repo">{repo}</div>
        </div>
      </div>
      <div className="main">
        <Page st={st} reload={reload} />
      </div>
    </div>
  );
}

export const Topbar = ({ title, sub, children }) => (
  <div className="topbar">
    <div>
      <h1>{title}</h1>
      <div className="sub">{sub}</div>
    </div>
    <div className="acts">{children}</div>
  </div>
);
