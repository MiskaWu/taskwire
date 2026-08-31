// 後端每個 API 一律回 {ok: bool, ...}，失敗帶 error（設計拍板三號）。
// Maybe<T> 就是這條約定的型別化：ok 當判別欄位，r.ok 一查就 narrow。
export interface ApiErr {
  ok: false;
  error?: string;
  manual?: string;
  text?: string;
}
export type Maybe<T> = (T & { ok: true }) | ApiErr;

// 寫入類動作的回應欄位聯集（各端點只回其中幾個）。
export interface OkPayload {
  text?: string;
  parsed?: string;
  restart_needed?: string[];
  shadowed?: string[];
  value?: string;
  threads?: Record<string, string>;
}
export type ActionResult = Maybe<OkPayload>;
export type Flash = ActionResult | { busy: true };

export interface UnitInfo {
  name: string;
  label: string;
  kind: string;
  help: string;
  note?: string;
  active: string;
  enabled: string;
  sub?: string;
  since?: string;
  next?: string;
  oncalendar?: string;
  detail?: string;
}

export interface SettingRow {
  key: string;
  label: string;
  default: string;
  type: string;
  help: string;
  restart: string[];
  value: string;
  effective: string;
  source: 'env' | 'file' | 'default';
}

export interface SecretRow {
  name: string;
  label: string;
  help: string;
  generate: string | null;
  present: boolean;
  length: number;
  hint: string;
  path: string;
  mode: string | null;
}

export interface CredsInfo {
  ok: boolean;
  label: string;
  hint: string;
}

export interface StateData {
  settings: SettingRow[];
  secrets: SecretRow[];
  units: UnitInfo[];
  systemd: boolean;
  threads: Record<string, string>;
  run_logs: string[];
  config_path: string;
  state_dir: string;
  repo_dir: string;
  webhook_url_hint: string;
  creds: CredsInfo;
}
export type StateResp = Maybe<StateData>;

export interface IssueRow {
  iid: number;
  title: string;
  url: string;
  labels: string[];
  updated_at: string;
}
export type IssuesResp = Maybe<{
  buckets: Record<string, IssueRow[]>;
  total: number;
  repo: string;
  fetched_at: string;
}>;

export type TokenResp = Maybe<{ expires_at: string; days: number | null }>;
export type LogResp = Maybe<{ name: string; text: string; truncated: boolean; total_lines: number }>;
export type SkillResp = Maybe<{ heading: string; body: string; path: string }>;
export type SecretValueResp = Maybe<{ name: string; value: string }>;
export type TextResp = Maybe<{ text: string }>;

// 頁面元件共用的 props：整頁狀態＋重新拉取。
export interface PageProps {
  st: StateResp | null;
  reload: () => void;
}
