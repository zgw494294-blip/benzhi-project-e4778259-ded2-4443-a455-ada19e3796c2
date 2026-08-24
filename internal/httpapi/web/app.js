const $ = (selector) => document.querySelector(selector);
let page = 1;
let createAttempt = null;
let revisionAttempt = null;
let currentArchive = null;
let selectedFinding = null;

async function api(url, options) {
  const response = await fetch(url, options);
  const data = await response.json();
  if (!response.ok) {
    const error = new Error(data.error || "请求失败");
    error.data = data;
    throw error;
  }
  return data;
}

function query() {
  const form = new FormData($("#filters"));
  const params = new URLSearchParams(Object.fromEntries(form));
  params.set("page", page);
  params.set("pageSize", "20");
  params.set("summary", "true");
  return params;
}

function archiveRow(archive) {
  const row = document.createElement("div");
  row.className = "archive";
  const label = document.createElement("span");
  const code = document.createElement("b");
  code.textContent = archive.archiveCode;
  label.append(code, document.createTextNode(` · ${archive.caveName} · ${archive.status}`));
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = "查看";
  button.onclick = () => show(archive.id);
  row.append(label, button);
  return row;
}

function renderSummary(summary) {
  if (!summary) {
    $("#summary").textContent = "";
    return;
  }
  const items = [
    `归档 ${summary.archiveCount}`,
    `修订 ${summary.revisionCount}`,
    `测站 ${summary.stationCount}`,
    `测段 ${summary.legCount}`,
    `有效凭据 ${summary.validCertificateCount}`,
    `未裁决 ${summary.unresolvedFindingCount}`,
    `待整改 ${summary.rectificationCount}`,
    `摘要异常 ${summary.frozenDigestAnomalyCount + summary.inconsistentCertificateCount}`,
  ];
  $("#summary").textContent = items.join(" · ");
  const risks = $("#risks");
  risks.replaceChildren();
  const ids = [...new Set([
    ...(summary.unresolvedArchiveIds || []),
    ...(summary.rectificationArchiveIds || []),
    ...(summary.frozenDigestAnomalyArchiveIds || []),
    ...(summary.inconsistentCertificateArchiveIds || []),
  ])];
  for (const id of ids) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = id;
    button.onclick = () => show(id);
    risks.append(button);
  }
}

async function load() {
  const data = await api("/api/archives?" + query());
  $("#counts").textContent = `共 ${data.total} 个归档包 · ` +
    Object.entries(data.statusCounts || {}).map(([key, value]) => `${key}:${value}`).join(" · ");
  const archives = $("#archives");
  archives.replaceChildren();
  if (data.archives.length) {
    data.archives.forEach((archive) => archives.append(archiveRow(archive)));
  } else {
    archives.textContent = "暂无归档包";
  }
  renderSummary(data.summary);
  $("#pager").innerHTML = `<button id="prev" ${page <= 1 ? "disabled" : ""}>上一页</button><span>第 ${data.page} 页</span><button id="next" ${page * data.pageSize >= data.total ? "disabled" : ""}>下一页</button>`;
  $("#prev").onclick = () => { page--; load(); };
  $("#next").onclick = () => { page++; load(); };
}

async function show(id) {
  currentArchive = await api("/api/archives/" + id);
  $("#detail").textContent = JSON.stringify(currentArchive, null, 2);
  const editable = currentArchive.status === "draft" || currentArchive.status === "rework";
  $("#revision-panel").hidden = !editable;
  $("#revision-context").textContent = `${currentArchive.archiveCode} · version ${currentArchive.version}`;
  const revisionForm = $("#revision-import");
  revisionForm.elements.parentRevisionId.value = currentArchive.status === "rework" ? currentArchive.currentRevisionId : "";
  revisionAttempt = null;
  $("#confirm-revision").disabled = true;
  $("#precheck-result").replaceChildren();

  const currentRuns = Object.values(currentArchive.checkRuns || {})
    .filter((run) => run.revisionId === currentArchive.currentRevisionId)
    .sort((a, b) => (a.completedAt === b.completedAt ? a.id.localeCompare(b.id) : a.completedAt.localeCompare(b.completedAt)));
  const comparisonAvailable = currentRuns.length > 0 && currentArchive.revisions[currentArchive.currentRevisionId]?.parentRevisionId;
  $("#comparison-panel").hidden = !comparisonAvailable;
  if (comparisonAvailable) {
    $("#comparison-filters").elements.checkRunId.value = currentRuns[currentRuns.length - 1].id;
  }
}

function revisionBody() {
  const form = Object.fromEntries(new FormData($("#revision-import")));
  form.expectedVersion = currentArchive.version;
  return form;
}

function renderErrors(target, errors) {
  const table = document.createElement("table");
  table.innerHTML = "<thead><tr><th>对象</th><th>来源行</th><th>字段</th><th>问题</th></tr></thead>";
  const body = document.createElement("tbody");
  for (const error of errors) {
    const row = document.createElement("tr");
    for (const value of [error.objectType, error.row || "-", error.field, error.reason]) {
      const cell = document.createElement("td");
      cell.textContent = value;
      row.append(cell);
    }
    body.append(row);
  }
  table.append(body);
  target.replaceChildren(table);
}

$("#precheck").onclick = async () => {
  if (!currentArchive) return;
  const body = revisionBody();
  try {
    const result = await api(`/api/archives/${currentArchive.id}/revisions/precheck`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!result.valid) {
      revisionAttempt = null;
      $("#confirm-revision").disabled = true;
      renderErrors($("#precheck-result"), result.errors || []);
      return;
    }
    revisionAttempt = { signature: JSON.stringify(body), key: crypto.randomUUID() };
    $("#confirm-revision").disabled = false;
    $("#precheck-result").textContent = `预检通过：${result.stationCount} 个测站，${result.legCount} 条测段，共 ${result.rowCount} 行`;
  } catch (error) {
    alert(error.message);
  }
};

$("#revision-import").addEventListener("input", () => {
  if (revisionAttempt && revisionAttempt.signature !== JSON.stringify(revisionBody())) {
    revisionAttempt = null;
    $("#confirm-revision").disabled = true;
  }
});

$("#revision-import").onsubmit = async (event) => {
  event.preventDefault();
  const body = revisionBody();
  if (!revisionAttempt || revisionAttempt.signature !== JSON.stringify(body)) return;
  try {
    const created = await api(`/api/archives/${currentArchive.id}/revisions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "Idempotency-Key": revisionAttempt.key },
      body: JSON.stringify(body),
    });
    revisionAttempt = null;
    await load();
    await show(created.id);
  } catch (error) {
    if (error.data?.errors) renderErrors($("#precheck-result"), error.data.errors);
    alert(error.message);
  }
};

function comparisonTable(report) {
  const table = document.createElement("table");
  table.innerHTML = "<thead><tr><th>分类</th><th>规则</th><th>严重度</th><th>对象</th><th>对象状态</th><th>原整改说明</th><th>变化</th><th>操作</th></tr></thead>";
  const labels = { resolved: "已解决", still_exists: "仍存在", new: "本次新增" };
  const body = document.createElement("tbody");
  for (const item of report.items) {
    const row = document.createElement("tr");
    const changes = (item.differences || []).map((difference) =>
      `${difference.objectType}:${difference.objectId} ${difference.change}`).join("；");
    for (const value of [labels[item.category], item.ruleCode, item.severity, `${item.subjectType}:${item.subjectId}`, item.objectStatus === "deleted" ? "已删除" : "存在", item.rectification || item.oldDecisionReason || "-", changes || "-"]) {
      const cell = document.createElement("td");
      cell.textContent = value;
      row.append(cell);
    }
    const action = document.createElement("td");
    if (item.currentFindingId) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = "裁决";
      button.onclick = () => {
        selectedFinding = item.currentFindingId;
        $("#decision-finding").textContent = `${item.ruleCode} · ${item.subjectId}`;
        $("#finding-decision").hidden = false;
      };
      action.append(button);
    }
    row.append(action);
    body.append(row);
  }
  table.append(body);
  return table;
}

$("#comparison-filters").onsubmit = async (event) => {
  event.preventDefault();
  const form = new FormData(event.target);
  const runID = form.get("checkRunId");
  form.delete("checkRunId");
  const params = new URLSearchParams(Object.fromEntries(form));
  try {
    const report = await api(`/api/archives/${currentArchive.id}/check-runs/${encodeURIComponent(runID)}/comparison?${params}`);
    $("#comparison-summary").textContent = `已解决 ${report.summary.resolved} · 仍存在 ${report.summary.stillExists} · 本次新增 ${report.summary.new}`;
    $("#comparison-result").replaceChildren(comparisonTable(report));
  } catch (error) {
    alert(error.message);
  }
};

$("#finding-decision").onsubmit = async (event) => {
  event.preventDefault();
  const body = Object.fromEntries(new FormData(event.target));
  body.expectedVersion = currentArchive.version;
  body.rectification = body.reason;
  try {
    await api(`/api/archives/${currentArchive.id}/findings/${selectedFinding}/decision`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    event.target.hidden = true;
    await show(currentArchive.id);
    $("#comparison-filters").requestSubmit();
  } catch (error) {
    alert(error.message);
  }
};

function certificateTable(result) {
  const table = document.createElement("table");
  table.innerHTML = "<thead><tr><th>凭据编号</th><th>结果</th><th>归档</th><th>签发人</th><th>签发时间</th><th>凭据哈希</th><th>清单引用</th><th>修订摘要</th><th>问题</th><th>查看</th></tr></thead>";
  const labels = { valid: "有效", invalid: "无效", not_found: "未找到" };
  const body = document.createElement("tbody");
  for (const item of result.items) {
    const row = document.createElement("tr");
    row.className = `result-${item.result}`;
    const values = [item.certificateId, labels[item.result], item.archiveId || "-", item.issuedBy || "-", item.issuedAt || "-", item.certificateHashOk ? "通过" : "失败", item.manifestReferenceOk ? "通过" : "失败", item.revisionHashOk ? "通过" : "失败", (item.problems || []).join("；") || "-"];
    for (const value of values) {
      const cell = document.createElement("td");
      cell.textContent = value;
      row.append(cell);
    }
    const action = document.createElement("td");
    if (item.archiveId) {
      const detail = document.createElement("button");
      detail.type = "button";
      detail.textContent = "详情";
      detail.onclick = () => show(item.archiveId);
      const timeline = document.createElement("button");
      timeline.type = "button";
      timeline.textContent = "时间线";
      timeline.onclick = async () => {
        const data = await api(`/api/archives/${item.archiveId}/timeline`);
        $("#detail").textContent = JSON.stringify(data, null, 2);
      };
      action.append(detail, timeline);
    }
    row.append(action);
    body.append(row);
  }
  table.append(body);
  return table;
}

$("#certificate-batch").onsubmit = async (event) => {
  event.preventDefault();
  const raw = new FormData(event.target).get("certificateIds");
  const certificateIds = raw.split(/\r?\n/).map((item) => item.trim()).filter(Boolean);
  try {
    const result = await api("/api/certificates/verify-batch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ certificateIds }),
    });
    $("#certificate-summary").textContent = `有效 ${result.summary.valid} · 无效 ${result.summary.invalid} · 未找到 ${result.summary.notFound}`;
    $("#certificate-result").replaceChildren(certificateTable(result));
  } catch (error) {
    alert(error.message);
  }
};

$("#create").onsubmit = async (event) => {
  event.preventDefault();
  const body = JSON.stringify(Object.fromEntries(new FormData(event.target)));
  if (!createAttempt || createAttempt.body !== body) {
    createAttempt = { body, key: crypto.randomUUID() };
  }
  try {
    const archive = await api("/api/archives", {
      method: "POST",
      headers: { "Content-Type": "application/json", "Idempotency-Key": createAttempt.key },
      body,
    });
    createAttempt = null;
    event.target.reset();
    page = 1;
    await load();
    await show(archive.id);
  } catch (error) {
    if (error.data?.existingArchiveId) await show(error.data.existingArchiveId);
    alert(error.message);
  }
};

$("#filters").onsubmit = (event) => {
  event.preventDefault();
  page = 1;
  load();
};
$("#refresh").onclick = load;
load();
