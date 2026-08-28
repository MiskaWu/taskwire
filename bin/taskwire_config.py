"""taskwire_config —— 設定的單一真相來源（Python 端）。

bash 端是 taskwire-config.sh，兩邊讀同一個 ~/.config/taskwire/config.env，
優先序也一致：**環境變數 > config.env > 這裡的 default**。

這支檔案同時是「旋鈕的目錄」：SETTINGS 一列就是一顆旋鈕，控制台的表單
直接由它長出來。以後要加旋鈕只加一筆，不必再去改網頁——旋鈕的定義散在
程式與頁面兩處，正是設定會慢慢對不上的起點。

restart 欄位記的是「改完這顆之後誰要重啟才生效」，控制台據此在存檔後
提示（或直接代跑）。空陣列代表下次呼叫自然生效，什麼都不用做。
"""

import os
import re

CONFIG_PATH = os.environ.get(
    "TASKWIRE_CONFIG", os.path.expanduser("~/.config/taskwire/config.env")
)
CONFIG_DIR = os.path.dirname(CONFIG_PATH)
STATE_DIR = os.environ.get(
    "TASKWIRE_STATE", os.path.expanduser("~/.local/state/taskwire")
)

# 密鑰不進 config.env：它們是 0600 的獨立檔，權限與內容都跟一般旋鈕不同性質。
SECRETS = [
    {
        "name": "webhook-secret",
        "label": "Webhook 密鑰",
        "help": "GitLab 專案 Webhook 設定裡的 Signing token 要填這個值。"
                "改了之後 GitLab 那端沒跟著改，門鈴就會聾掉（403）。",
        "generate": "hex32",
    },
    {
        "name": "discord-webhook",
        "label": "Discord Webhook 網址",
        "help": "推播的落點，必須是**論壇頻道**的 webhook——一般文字頻道開不了討論串"
                "（Discord 回 220003），通知會退化成不分串。留空則完全不推 Discord。",
        "generate": None,
    },
]

SETTINGS = [
    {
        "key": "TASK_REPO",
        "label": "Backlog 倉庫路徑",
        "default": "gitlab.dev.baasgames.com/hydrogen/roles/engineer/miskawu/docs/backlogs",
        "type": "text",
        "help": "**必須帶主機名**。只寫 <group>/<repo> 的話 glab 會靜默打 gitlab.com，"
                "症狀是 404／401 而不是「找不到指令」。",
        "restart": [],
    },
    {
        "key": "TASK_DISPATCH_TIMEOUT",
        "label": "無頭取件逾時（秒）",
        "default": "3600",
        "type": "number",
        "help": "單次無頭會話的上限，防「做錯方向做一整夜」。逾時會被 timeout 殺掉，"
                "機械兜底接手把單子補成 blocked。",
        "restart": [],
    },
    {
        "key": "TASK_WEBHOOK_PORT",
        "label": "門鈴監聽埠",
        "default": "9587",
        "type": "number",
        "help": "改這個要同步改 GitLab 那端的 Webhook URL 與 Windows 防火牆入站規則。",
        "restart": ["task-webhook.service"],
    },
    {
        "key": "TASK_UI_PORT",
        "label": "控制台埠",
        "default": "9588",
        "type": "number",
        "help": "這個控制台自己的埠，只綁 127.0.0.1。改完要重啟控制台，"
                "重啟後這一頁的網址也跟著換。",
        "restart": ["task-ui.service"],
    },
    {
        "key": "TASK_UI_BIND",
        "label": "控制台綁定位址",
        "default": "127.0.0.1",
        "type": "text",
        "help": "預設只綁本機。頁面上有 webhook 密鑰與服務控制權，而且是 http 明文，"
                "**不建議改成 0.0.0.0**——真要區網存取請先想清楚誰在同一個網段。",
        "restart": ["task-ui.service"],
    },
]

SETTING_KEYS = {s["key"] for s in SETTINGS}
DEFAULTS = {s["key"]: s["default"] for s in SETTINGS}

_LINE = re.compile(r"^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$")


def _unquote(v: str) -> str:
    if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'":
        return v[1:-1]
    return v


def read_file() -> dict:
    """只讀 config.env 的內容，不套環境變數也不套預設值。
    控制台編輯表單要顯示的是「檔案裡實際寫了什麼」，不是「跑起來會拿到什麼」。"""
    out = {}
    try:
        with open(CONFIG_PATH, encoding="utf-8") as fh:
            for line in fh:
                if not line.strip() or line.lstrip().startswith("#"):
                    continue
                m = _LINE.match(line)
                if m:
                    out[m.group(1)] = _unquote(m.group(2))
    except OSError:
        pass
    return out


def effective() -> dict:
    """實際生效的值，套用完整優先序：環境變數 > config.env > default。
    跟 taskwire-config.sh 的行為必須一致——兩邊分岔的話，控制台顯示的
    就不是腳本真正在用的值，那比沒有控制台更糟。"""
    on_disk = read_file()
    out = {}
    for key, default in DEFAULTS.items():
        out[key] = os.environ.get(key) or on_disk.get(key) or default
    for key, val in on_disk.items():
        out.setdefault(key, val)
    return out


def source_of(key: str) -> str:
    """這個值現在是從哪裡來的——控制台要標示出來，
    否則你在網頁上改了卻不生效（因為被環境變數蓋住）會找不到原因。"""
    if os.environ.get(key):
        return "env"
    if key in read_file():
        return "file"
    return "default"


def write_file(values: dict) -> None:
    """整檔重寫，帶說明註解。認識的旋鈕照 SETTINGS 的順序排並附註解，
    不認識的 key 原樣保留在檔尾——人手加的東西不該被控制台默默吃掉。"""
    os.makedirs(CONFIG_DIR, exist_ok=True)
    known = {s["key"]: s for s in SETTINGS}
    lines = [
        "# taskwire 設定。控制台（task-ui）讀寫這個檔，手改也可以。",
        "# 優先序：環境變數 > 這個檔 > 腳本內建預設。",
        "# 純 KEY=value，不做 shell 展開——這是設定檔，不是腳本。",
        "",
    ]
    for setting in SETTINGS:
        key = setting["key"]
        if key not in values:
            continue
        help_text = re.sub(r"\*\*(.+?)\*\*", r"\1", setting["help"])
        lines.append(f"# {setting['label']}：{help_text}")
        lines.append(f"{key}={values[key]}")
        lines.append("")
    extra = {k: v for k, v in values.items() if k not in known}
    if extra:
        lines.append("# —— 以下不是控制台認識的旋鈕，原樣保留 ——")
        for key in sorted(extra):
            lines.append(f"{key}={extra[key]}")
        lines.append("")
    tmp = CONFIG_PATH + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write("\n".join(lines))
    os.replace(tmp, CONFIG_PATH)


def secret_path(name: str) -> str:
    return os.path.join(CONFIG_DIR, name)


def read_secret(name: str) -> str:
    try:
        with open(secret_path(name), encoding="utf-8") as fh:
            return fh.read().strip()
    except OSError:
        return ""


def write_secret(name: str, value: str) -> None:
    """0600 寫入。先建檔設權限再寫內容，避免有一瞬間是 0644 的密鑰檔。"""
    os.makedirs(CONFIG_DIR, exist_ok=True)
    path = secret_path(name)
    tmp = path + ".tmp"
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as fh:
        fh.write(value.strip() + "\n")
    os.replace(tmp, path)
