import { useEffect, useState, useCallback } from 'react';
import type { ComponentType, ReactNode } from 'react';
import { api } from './api';
import { Icon, Dot } from './ui';
import type { PageProps, StateData, StateResp } from './types';
import Overview from './pages/Overview';
import Services from './pages/Services';
import Tickets from './pages/Tickets';
import Logs from './pages/Logs';
import Settings from './pages/Settings';

const NAV: Array<[string, string, string]> = [
  ['overview', '總覽', 'gauge'],
  ['services', '服務與排程', 'server'],
  ['tickets', '單況', 'board'],
  ['logs', '紀錄', 'file'],
  ['settings', '設定與密鑰', 'sliders'],
];

const PAGES: Record<string, ComponentType<PageProps>> = {
  overview: Overview, services: Services, tickets: Tickets, logs: Logs, settings: Settings,
};

export default function App() {
  const [route, setRoute] = useState(location.hash.slice(1) || 'overview');
  const [st, setSt] = useState<StateResp | null>(null);

  const reload = useCallback(async () => setSt(await api<StateData>('/api/state')), []);
  useEffect(() => {
    reload();
    const t = setInterval(reload, 15000); // 狀態每 15 秒背景刷新
    const onHash = () => setRoute(location.hash.slice(1) || 'overview');
    addEventListener('hashchange', onHash);
    return () => { clearInterval(t); removeEventListener('hashchange', onHash); };
  }, [reload]);

  const allUp = st?.ok && st.units.every((u) => u.active === 'active');
  const repo = st?.ok ? (st.settings.find((s) => s.key === 'TASK_REPO')?.effective || '') : '';
  const Page = PAGES[route] || Overview;

  return (
    <div className="shell">
      <div className="side">
        <div className="brand">
          <Dot c={allUp ? '#5ed99a' : '#f0c060'} />
          <div>
            <div className="name">TaskWire</div>
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
            <Dot c={st ? (st.ok && st.systemd ? '#5ed99a' : '#f0c060') : '#6b7484'} />
            {st ? (st.ok && st.systemd ? 'systemd 可用' : 'systemd 降級模式') : '載入中…'}
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

export const Topbar = ({ title, sub, children }: { title: ReactNode; sub?: ReactNode; children?: ReactNode }) => (
  <div className="topbar">
    <div>
      <h1>{title}</h1>
      <div className="sub">{sub}</div>
    </div>
    <div className="acts">{children}</div>
  </div>
);
