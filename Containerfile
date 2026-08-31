# taskwire 控制台的容器版。
#
# 先講清楚這份檔案的性質：控制台的工作定義就是「操作宿主機」——讀寫宿主機的設定、
# 開關宿主機的 systemd unit、讀宿主機的 journal、拿宿主機的 GitLab token 查單況。
# 容器化一個以操作宿主機為業的服務，得把這些能力一項一項 mount 回去，
# 每 mount 一項就削掉一塊隔離。所以這個映像刻意做成「你 mount 多少，它就能做多少」：
# 少給的部分不會壞掉，只會降級成在頁面上把該跑的指令印給你複製（見 hostAction）。
#
# 控制台本體是 Go 靜態執行檔（2026-08-31 改寫；前端已 go:embed 在裡面），
# 由掛進來的 repo volume 提供（bin/task-ui，先在宿主機 make 好）。
# 裝的東西分兩層，愈往下愈是為了容器化本身而付的代價：
#   1. bash jq curl   —— task／task-notify／task-doctor 這些腳本要用。
#   2. glab, systemd  —— 查單況要 glab；按得動「重啟門鈴」要 systemctl 與 journalctl，
#                        而它們只存在於 systemd 套件裡。第 2 層是純粹的容器稅：
#                        直跑的話這兩樣宿主機上本來就有。
FROM docker.io/library/debian:stable-slim

ARG GLAB_VERSION=1.53.0
ARG TARGETARCH=amd64

RUN apt-get update && apt-get install -y --no-install-recommends \
      bash jq curl ca-certificates \
 && curl -fsSL -o /tmp/glab.deb \
      "https://gitlab.com/gitlab-org/cli/-/releases/v${GLAB_VERSION}/downloads/glab_${GLAB_VERSION}_linux_${TARGETARCH}.deb" \
 && apt-get install -y --no-install-recommends /tmp/glab.deb \
 && rm /tmp/glab.deb \
 && apt-get clean && rm -rf /var/lib/apt/lists/*

# 第 2 層：只為了讓頁面上的「重啟／停止／看 journal」真的按得動。
# 不需要那幾顆按鈕的話（例如你只想用控制台改設定與看單況），把這一段註解掉，
# 映像會小掉數十 MB，那些動作自動降級成印指令給你複製。
RUN apt-get update && apt-get install -y --no-install-recommends systemd \
 && apt-get clean && rm -rf /var/lib/apt/lists/*

# 路徑跟宿主機對齊。搭配 podman 的 --userns=keep-id，容器內外是同一個 uid、
# 同一組路徑——D-Bus 的 socket 認 uid，路徑錯開的話 systemctl --user 一定連不上。
ENV HOME=/home/miskawu \
    TASKWIRE_REPO_DIR=/home/miskawu/projects/taskwire \
    PATH=/home/miskawu/projects/taskwire/bin:/usr/local/bin:/usr/bin:/bin

WORKDIR /home/miskawu/projects/taskwire
EXPOSE 9588
CMD ["/home/miskawu/projects/taskwire/bin/task-ui"]
