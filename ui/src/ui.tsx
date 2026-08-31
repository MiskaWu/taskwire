// 共用元件——設計稿 2026-08-28 的元件語言
import type { CSSProperties, ReactNode, ButtonHTMLAttributes } from 'react';

export const Badge = ({ k = 'idle', children }: { k?: string; children: ReactNode }) => (
  <span className={`badge b-${k}`}>{children}</span>
);

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & { kind?: string };
export const Btn = ({ kind = '', children, ...rest }: BtnProps) => (
  <button className={`btn ${kind}`} {...rest}>{children}</button>
);

export const Dot = ({ c }: { c: string }) => <span className="dot" style={{ background: c }} />;
export const Spin = () => <span className="spin" />;

export const Card = ({ title, sub, right, className = '', style, children }: {
  title?: ReactNode; sub?: ReactNode; right?: ReactNode;
  className?: string; style?: CSSProperties; children?: ReactNode;
}) => (
  <div className={`card ${className}`} style={style}>
    {(title || right) && (
      <div className="card-head">
        <div>
          <div className="t">{title}</div>
          {sub && <div className="s">{sub}</div>}
        </div>
        {right && <div className="right">{right}</div>}
      </div>
    )}
    {children}
  </div>
);

// 失敗永遠長成紅框，絕不渲染成空清單——「查不到」和「沒有」在畫面上必須分得出來。
export const ErrBox = ({ r }: { r: { ok?: boolean; error?: string; manual?: string; text?: string } | null | undefined }) => {
  if (!r || r.ok) return null;
  return (
    <div className="err">
      <strong>{r.error || '失敗'}</strong>
      {r.manual && (
        <div style={{ marginTop: 6 }}>
          <pre>{r.manual}</pre>
          <Btn onClick={() => navigator.clipboard.writeText(r.manual!)}>複製指令</Btn>
        </div>
      )}
      {r.text && <pre>{r.text}</pre>}
    </div>
  );
};

export const Tile = ({ label, badge, value, mono, sub }: {
  label: ReactNode; badge?: ReactNode; value: ReactNode; mono?: boolean; sub?: ReactNode;
}) => (
  <div className="tile">
    <div className="lab"><span>{label}</span>{badge}</div>
    <div className={`val ${mono ? 'mono' : ''}`}>{value}</div>
    <div className="sub">{sub}</div>
  </div>
);

const P = { fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' } as const;
export const Icon = ({ name, size = 18 }: { name: string; size?: number }) => {
  const body: Record<string, ReactNode> = {
    gauge: <><path d="M12 14l3.5-4.5" /><path d="M20.2 16a8.5 8.5 0 1 0-16.4 0" /></>,
    server: <><rect x="3.5" y="4.5" width="17" height="6.5" rx="1.5" /><rect x="3.5" y="13" width="17" height="6.5" rx="1.5" /><path d="M7 7.75h.01M7 16.25h.01" /></>,
    board: <path d="M5 4.5v15M12 4.5v10M19 4.5v6" />,
    file: <><path d="M6 3.5h8l4 4v13H6z" /><path d="M14 3.5v4h4M9 12h6M9 15.5h6" /></>,
    sliders: <><path d="M4 7h9M17 7h3M4 12h3M11 12h9M4 17h9M17 17h3" /><circle cx="15" cy="7" r="2" /><circle cx="9" cy="12" r="2" /><circle cx="15" cy="17" r="2" /></>,
  };
  return <svg viewBox="0 0 24 24" width={size} height={size} {...P}>{body[name]}</svg>;
};

export const days = (ts?: string): number | '' =>
  ts ? Math.floor((Date.now() - new Date(ts).getTime()) / 86400000) : '';
