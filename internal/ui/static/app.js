// fixmyrouter local UI. Plain JS, no build step, no network calls except to
// this local server. All text from the API is inserted with textContent.
"use strict";

const token = document.querySelector('meta[name="fixmyrouter-token"]').content;
const $ = (id) => document.getElementById(id);

// h builds a DOM element: h("p", {class: "x"}, "text", child, ...)
function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "style") el.style.cssText = v; // CSSOM, allowed by our CSP
    else el.setAttribute(k, v);
  }
  for (const kid of kids.flat()) {
    if (kid !== null && kid !== undefined && kid !== false) el.append(kid);
  }
  return el;
}

async function api(path, body) {
  const opts = { headers: { "X-Fixmyrouter-Token": token } };
  if (body !== undefined) {
    opts.method = "POST";
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({ error: res.statusText }));
  if (!res.ok || data.error) throw new Error(data.error || res.statusText);
  return data;
}

function setStatus(msg, isError) {
  $("status").textContent = msg;
  $("status").classList.toggle("error", !!isError);
}

const gradeColor = (s) => (s >= 90 ? "var(--pass)" : s >= 75 ? "var(--accent)" : s >= 50 ? "var(--warn)" : "var(--fail)");
const when = (iso) => new Date(iso).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
const kindName = { check: "Full check", speed: "Speed", audit: "Security" };

function renderScore(latest, previous) {
  const arcLen = 235.6; // 270 degrees of a r=50 circle
  $("score").textContent = latest.score;
  $("score-label").textContent = latest.grade;
  $("arc").style.strokeDasharray = `${(arcLen * latest.score) / 100} 314.2`;
  $("arc").style.stroke = gradeColor(latest.score);
  const problems = latest.findings.filter((f) => f.status === "warn" || f.status === "fail");
  $("summary").textContent =
    `${kindName[latest.kind] || "Check"} from ${when(latest.generated_at)}. ` +
    (problems.length === 0 ? "Nothing obvious to fix." : `${problems.length} thing${problems.length > 1 ? "s" : ""} worth fixing, worst first below.`);
  if (latest.gateway) {
    $("gateway").hidden = false;
    $("gateway").textContent = `Router ${latest.gateway.ip}${latest.gateway.vendor ? " · " + latest.gateway.vendor : ""}`;
  }
  const cmp = $("compare");
  cmp.replaceChildren();
  cmp.hidden = !previous;
  if (!previous) return;
  const d = latest.score - previous.score;
  const prevIDs = new Set(previous.findings.filter(isProblem).map((f) => f.id));
  const nowIDs = new Set(problems.map((f) => f.id));
  const fixed = [...prevIDs].filter((id) => !nowIDs.has(id)).length;
  const added = [...nowIDs].filter((id) => !prevIDs.has(id)).length;
  cmp.append(
    h("span", {}, "Before ", h("b", {}, String(previous.score))),
    h("span", { "aria-hidden": "true" }, "→"),
    h("span", {}, "After ", h("b", {}, String(latest.score))),
    h("span", { class: "delta " + (d > 0 ? "up" : d < 0 ? "down" : "") }, d === 0 ? "no change" : (d > 0 ? "+" : "") + d),
    h("span", { class: "muted" }, `${fixed} fixed, ${added} new since ${when(previous.generated_at)}`),
  );
}

const isProblem = (f) => f.status === "warn" || f.status === "fail";

function metric(title, big, sub, extra) {
  return h("section", { class: "card metric", "aria-label": title }, h("h2", {}, title), h("div", { class: "big" }, big), h("div", { class: "sub" }, sub), extra);
}

function renderMeasurements(sp) {
  const box = $("measurements");
  box.replaceChildren();
  box.hidden = !sp;
  if (!sp) return;
  const w = sp.wifi;
  if (w) {
    const quality = w.signal_dbm >= -60 ? "excellent" : w.signal_dbm >= -67 ? "good" : w.signal_dbm >= -75 ? "fair" : "weak";
    box.append(metric("Wi-Fi", `${w.band} · ch ${w.channel}`,
      [w.signal_dbm ? `Signal ${w.signal_dbm} dBm (${quality})` : "", w.width_mhz ? `${w.width_mhz} MHz` : "", w.phy_mode || "", w.security ? w.security.toUpperCase() : ""].filter(Boolean).join(" · "),
      sp.congestion ? h("div", { class: "sub" }, `${sp.congestion.overlapping_my_channel} neighbour network(s) on your channel; quietest: ${sp.congestion.quietest_channel}`) : null));
  } else {
    box.append(metric("Wi-Fi", "–", sp.wifi_note || "No Wi-Fi details"));
  }
  const g = sp.gateway_latency;
  box.append(g
    ? metric("Router latency", `${g.median_ms} ms`, `jitter ${g.jitter_ms} ms · loss ${g.loss_pct}% · ${g.sent} pings`)
    : metric("Router latency", "–", "Your router did not answer ping (some block it)."));
  if (sp.dns && sp.dns.length) {
    const ok = sp.dns.filter((d) => d.failures < d.queries);
    const max = Math.max(1, ...ok.map((d) => d.median_ms));
    box.append(h("section", { class: "card metric", "aria-label": "DNS lookup time" }, h("h2", {}, "DNS lookup time"),
      h("ul", { class: "bars" }, sp.dns.map((d) => {
        const dead = d.failures >= d.queries;
        return h("li", { class: d.system ? "mine" : "" },
          h("span", { title: d.server }, d.system ? "Yours" : d.name),
          h("span", { class: "bar", role: "img", "aria-label": dead ? "no answer" : `${d.median_ms} ms` }, h("span", { style: `width:${dead ? 0 : Math.max(3, (100 * d.median_ms) / max)}%` })),
          h("span", { class: "ms" }, dead ? "none" : `${d.median_ms} ms`));
      }))));
  }
  const b = sp.bufferbloat;
  box.append(b
    ? metric("Bufferbloat", `Grade ${b.grade}`, `+${b.increase_ms} ms under load (${b.idle.median_ms} → ${b.loaded.median_ms} ms) · ~${b.download_mbps} Mbps down`)
    : metric("Bufferbloat", "–", sp.bufferbloat_note && !sp.bufferbloat_note.startsWith("Skipped") ? sp.bufferbloat_note : "Not tested. Tick “Include bufferbloat test” and run again."));
}

function findingItem(f) {
  return h("li", { class: "finding " + f.status },
    h("div", { class: "finding-head" },
      h("span", { class: "badge " + (isProblem(f) ? f.severity : f.status) }, isProblem(f) ? f.severity : f.status === "pass" ? "ok" : "could not run"),
      h("h3", {}, f.title)),
    h("p", {}, f.detail),
    f.fix ? h("p", { class: "how muted" }, h("strong", {}, "How to fix: "), f.fix) : null,
    f.router_steps ? h("p", { class: "how" }, h("strong", {}, "Your router: "), f.router_steps) : null,
    f.auto_fix ? h("div", { class: "actions" },
      h("button", { class: "primary small", type: "button", onclick: () => confirmFix(f) }, "Apply this fix…"),
      h("span", { class: "muted" }, f.auto_fix.summary)) : null);
}

function renderFindings(latest) {
  const list = $("finding-list");
  list.replaceChildren();
  $("findings").hidden = false;
  const problems = latest.findings.filter((f) => f.status !== "pass");
  const passes = latest.findings.filter((f) => f.status === "pass");
  if (problems.length === 0) list.append(h("li", { class: "muted" }, "Nothing to fix. Nice."));
  list.append(...problems.map(findingItem));
  if (passes.length) {
    list.append(h("li", {}, h("details", { class: "passes" },
      h("summary", {}, `${passes.length} check${passes.length > 1 ? "s" : ""} passed`),
      h("ul", { class: "findings" }, passes.map(findingItem)))));
  }
}

function renderHistory(scores) {
  const box = $("sparkline");
  if (!scores.length) return;
  const W = 300, H = 80, pad = 6;
  const pts = scores.map((s, i) => [pad + (scores.length === 1 ? (W - 2 * pad) / 2 : (i * (W - 2 * pad)) / (scores.length - 1)), H - pad - ((H - 2 * pad) * s.score) / 100]);
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", `0 0 ${W} ${H}`);
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "Scores: " + scores.map((s) => `${s.score} on ${when(s.at)}`).join(", "));
  const line = document.createElementNS(ns, "polyline");
  line.setAttribute("points", pts.map((p) => p.join(",")).join(" "));
  svg.append(line);
  for (const [x, y] of pts) {
    const c = document.createElementNS(ns, "circle");
    c.setAttribute("cx", x); c.setAttribute("cy", y); c.setAttribute("r", 3);
    svg.append(c);
  }
  const last = scores[scores.length - 1];
  box.replaceChildren(svg, h("p", { class: "muted" }, `${scores.length} result${scores.length > 1 ? "s" : ""}, latest ${last.score}/100`));
}

function renderChanges(changes) {
  const list = $("changes");
  list.replaceChildren();
  if (!changes.length) {
    list.append(h("li", { class: "muted" }, "fixmyrouter has never changed anything."));
    return;
  }
  for (const c of [...changes].reverse()) {
    const canUndo = c.status !== "undone";
    list.append(h("li", {},
      h("div", {}, h("div", {}, `#${c.id} ${c.summary}`),
        h("div", { class: "when" }, `${c.status} · ${when(c.applied_at)} · before: ${c.before.length ? c.before.join(", ") : "automatic"}`)),
      canUndo ? h("button", { class: "secondary small", type: "button", onclick: () => undo(c.id) }, "Undo") : null));
  }
}

async function confirmFix(f) {
  setStatus("Reading your current setting (nothing is changed yet)…");
  let plan;
  try {
    plan = await api("/api/fix/plan", { finding_id: f.id });
  } catch (e) {
    setStatus(e.message, true);
    return;
  }
  setStatus("");
  $("confirm-summary").textContent = f.auto_fix.summary;
  $("plan-service").textContent = plan.service;
  $("plan-before").textContent = plan.before.length ? plan.before.join(", ") : "automatic (from your router)";
  $("plan-after").textContent = plan.after.join(", ");
  const dlg = $("confirm");
  dlg.returnValue = "";
  dlg.showModal();
  dlg.addEventListener("close", async () => {
    if (dlg.returnValue !== "yes") {
      setStatus("Nothing changed.");
      return;
    }
    setStatus("Applying…");
    try {
      const c = await api("/api/fix/apply", { finding_id: f.id });
      setStatus(`Done (change #${c.id}). Run a speed check to compare, or undo below.`);
    } catch (e) {
      setStatus("The change failed: " + e.message + " Use Undo below to make sure the old setting is back.", true);
    }
    load();
  }, { once: true });
}

async function undo(id) {
  setStatus("Undoing…");
  try {
    const c = await api("/api/undo", { id });
    setStatus(`Undid change #${c.id}. DNS is back to ${c.before.length ? c.before.join(", ") : "automatic"}.`);
  } catch (e) {
    setStatus(e.message, true);
  }
  load();
}

async function runCheck(kind) {
  const buttons = document.querySelectorAll("button[data-kind]");
  buttons.forEach((b) => (b.disabled = true));
  const withLoad = $("load").checked && kind !== "audit";
  setStatus(withLoad ? "Measuring, including the download test (about 25 seconds)…" : "Measuring (about 10 seconds)…");
  try {
    await api("/api/run", { kind, load: withLoad });
    setStatus("");
    await load();
  } catch (e) {
    setStatus(e.message, true);
  } finally {
    buttons.forEach((b) => (b.disabled = false));
  }
}

async function load() {
  try {
    const st = await api("/api/state");
    $("version").textContent = "v" + st.version;
    if (st.latest) {
      renderScore(st.latest, st.previous);
      renderMeasurements(st.latest.speed);
      renderFindings(st.latest);
    }
    if (typeof renderDiagnosis === "function") renderDiagnosis(st.diagnosis);
    renderHistory(st.scores);
    renderChanges(st.changes);
  } catch (e) {
    setStatus(e.message, true);
  }
}

function applyTheme(t) {
  document.documentElement.dataset.theme = t;
  $("theme").textContent = t === "dark" ? "☀︎" : "☾";
  $("theme").setAttribute("aria-label", t === "dark" ? "Switch to light theme" : "Switch to dark theme");
}

document.querySelectorAll("button[data-kind]").forEach((b) => b.addEventListener("click", () => runCheck(b.dataset.kind)));
$("theme").addEventListener("click", () => {
  const t = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
  applyTheme(t);
  try { localStorage.setItem("theme", t); } catch (_) { /* private mode */ }
});
try { applyTheme(localStorage.getItem("theme") || "dark"); } catch (_) { applyTheme("dark"); }
load();
