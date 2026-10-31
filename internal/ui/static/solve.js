// Diagnose, fix and measure, ISP report. Loaded after app.js (uses h, api, $).
"use strict";

const levelName = {
  auto: "fixmyrouter can do this",
  router: "on your router (terminal)",
  guided: "step by step",
  advice: "what to do",
};

function setDStatus(id, msg, isError) {
  $(id).textContent = msg;
  $(id).classList.toggle("error", !!isError);
}

function renderDiagnosis(d) {
  const box = $("verdict");
  box.hidden = !d;
  if (!d) return;
  box.classList.toggle("healthy", d.healthy);
  $("verdict-text").textContent = d.verdict;
  $("verdict-when").textContent = `Measured ${when(d.generated_at)}` + (d.router_profile ? ` · router profile ${d.router_profile}` : "");
  const list = $("bottlenecks");
  list.replaceChildren();
  for (const b of d.bottlenecks) {
    const main = b.kind === d.primary;
    list.append(h("article", { class: "bottleneck" + (main ? " main" : "") },
      h("div", { class: "finding-head" },
        h("span", { class: "badge " + (main ? "fail" : "warn") }, main ? "main bottleneck" : "also"),
        h("h3", {}, b.name),
        h("span", { class: "muted" }, `impact ${b.impact}/100`)),
      h("p", {}, b.headline),
      b.explain ? h("p", { class: "muted" }, b.explain) : null,
      h("ul", { class: "evidence" }, b.evidence.map((e) => h("li", {}, h("b", {}, e.name + ": "), e.value, e.note ? h("span", { class: "muted" }, ` (${e.note})`) : null))),
      b.actions.length ? h("div", { class: "fixes" }, h("h4", {}, "How to fix it"), b.actions.map(actionItem)) : null));
  }
  const notes = $("notes");
  notes.replaceChildren(...(d.notes || []).map((n) => h("li", {}, n)));
}

function actionItem(a) {
  let button = null;
  if (a.level === "auto") {
    button = h("button", { class: "primary small", type: "button", onclick: () => fixAuto(a) }, "Fix it and measure");
  } else if (a.level === "router") {
    button = h("p", { class: "muted small" }, "Needs your router password, typed in a terminal (the browser never sees it): ", h("code", {}, `fixmyrouter fix --measure ${a.id}`));
  } else if (a.metric) {
    button = h("button", { class: "secondary small", type: "button", onclick: () => fixGuided(a) }, "I've done this: measure again");
  }
  return h("details", { class: "fix " + a.level, open: a.level === "auto" || a.level === "guided" ? "" : null },
    h("summary", {}, h("span", { class: "badge level-" + a.level }, levelName[a.level] || a.level), " ", a.title),
    a.steps && a.steps.length ? h("ol", {}, a.steps.map((s) => h("li", {}, s))) : null,
    a.command && a.level !== "router" ? h("p", { class: "muted small" }, "Command line: ", h("code", {}, a.command)) : null,
    button);
}

async function runDiagnose() {
  const btn = $("diagnose");
  btn.disabled = true;
  const body = {
    throughput: $("d-throughput").checked,
    load: $("d-load").checked,
    plan_mbps: Number($("d-plan").value) || 0,
  };
  let secs = 10 + (body.throughput ? 12 : 0) + (body.load ? 12 : 0);
  setDStatus("d-status", `Measuring (about ${secs} seconds)…`);
  try {
    const d = await api("/api/diagnose", body);
    renderDiagnosis(d);
    setDStatus("d-status", "");
    $("verdict").scrollIntoView({ behavior: "smooth", block: "start" });
  } catch (e) {
    setDStatus("d-status", e.message, true);
  } finally {
    btn.disabled = false;
  }
}

async function needsTest(a) {
  const p = await api("/api/fix/preview", { fix_id: a.id });
  if (p.missing_test) {
    setDStatus("d-status", `To measure this fix before and after, tick the ${p.missing_test === "--load" ? "lag-under-load" : "download speed"} test and Diagnose again first.`, true);
    return null;
  }
  return p;
}

async function fixAuto(a) {
  let p;
  try {
    p = await needsTest(a);
  } catch (e) {
    setDStatus("d-status", e.message, true);
    return;
  }
  if (!p) return;
  const c = p.change;
  $("fix-summary").textContent = a.auto_fix.summary;
  $("fix-service").textContent = c.service;
  $("fix-before").textContent = c.before && c.before.length ? c.before.join(", ") : "automatic (from your router)";
  $("fix-after").textContent = c.after.join(", ");
  const dlg = $("fix-confirm");
  dlg.returnValue = "";
  dlg.showModal();
  dlg.addEventListener("close", () => {
    if (dlg.returnValue !== "yes") {
      setDStatus("d-status", "Nothing changed.");
      return;
    }
    measureFix(a, "Applying, then measuring again…");
  }, { once: true });
}

async function fixGuided(a) {
  try {
    if (!(await needsTest(a))) return;
  } catch (e) {
    setDStatus("d-status", e.message, true);
    return;
  }
  measureFix(a, "Measuring again…");
}

async function measureFix(a, msg) {
  setDStatus("d-status", msg);
  try {
    const res = await api("/api/fix/measure", { fix_id: a.id, confirmed: true });
    renderBeforeAfter(res);
    renderDiagnosis(res.after);
    setDStatus("d-status", "");
  } catch (e) {
    setDStatus("d-status", e.message, true);
  }
  load();
}

const num = (v) => (Number.isInteger(v) ? String(v) : v.toFixed(1));

function renderBeforeAfter(res) {
  const box = $("beforeafter");
  box.hidden = false;
  $("ba-action").textContent = `Fix: ${res.action.title}`;
  $("ba-rows").replaceChildren(...res.delta.metrics.map((m) => {
    const u = m.unit ? " " + m.unit : "";
    return h("tr", { class: m.better ? "better" : m.worse ? "worse" : "" },
      h("th", { scope: "row" }, (m.target ? "★ " : "") + m.name),
      h("td", {}, num(m.before) + u), h("td", {}, num(m.after) + u),
      h("td", {}, (m.change > 0 ? "+" : "") + num(m.change) + u + (m.better ? " · better" : m.worse ? " · worse" : "")));
  }));
  $("ba-verdict").textContent = res.delta.verdict;
  $("ba-verdict").classList.toggle("good", res.delta.improved);
  const undoBox = $("ba-undo");
  undoBox.replaceChildren();
  if (res.change_id) {
    undoBox.append(h("button", { class: "secondary small", type: "button", onclick: () => undo(res.change_id) }, `Undo this change (#${res.change_id})`));
  }
  box.scrollIntoView({ behavior: "smooth", block: "start" });
}

function download(name, type, text) {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = h("a", { href: url, download: name });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

async function ispReport() {
  const samples = Number($("r-samples").value) || 1;
  const interval = Number($("r-interval").value) || 60;
  const noDL = $("r-nodl").checked;
  const mins = Math.round(((samples - 1) * interval) / 60);
  const data = noDL ? "almost no data" : `up to ${samples * 25} MB of downloads`;
  if (!confirm(`Take ${samples} sample(s), every ${interval} s (about ${mins} min), using ${data}. Keep this page open. Start?`)) return;
  const btn = $("isp-report");
  btn.disabled = true;
  setDStatus("r-status", `Measuring… about ${mins} min. Keep this page open.`);
  try {
    const r = await api("/api/report/isp", {
      samples, interval_seconds: interval, no_download: noDL, confirmed: true,
      plan_mbps: Number($("d-plan").value) || 0,
    });
    download(r.filename + ".md", "text/markdown", r.markdown);
    download(r.filename + ".json", "application/json", JSON.stringify(r.report, null, 2));
    setDStatus("r-status", `${r.report.conclusion} Downloaded, and saved in ${r.markdown_file}.`);
  } catch (e) {
    setDStatus("r-status", e.message, true);
  } finally {
    btn.disabled = false;
  }
}

$("diagnose").addEventListener("click", runDiagnose);
$("isp-report").addEventListener("click", ispReport);
