package httpapi

import (
	"net/http"
)

func (api API) dashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(dashboardHTML))
}

func (api API) metrics(w http.ResponseWriter, r *http.Request) {
	if api.eventStore == nil {
		writeError(w, http.StatusServiceUnavailable, "event store is not configured")
		return
	}

	metrics, err := api.eventStore.DashboardMetrics(r.Context())
	if err != nil {
		api.logger.Error("failed to load dashboard metrics", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load metrics")
		return
	}

	writeJSON(w, http.StatusOK, metrics)
}

const dashboardHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Mentoria - Métricas</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f7f9;
      --panel: #ffffff;
      --line: #d9dee7;
      --text: #18202f;
      --muted: #677286;
      --ok: #197b55;
      --warn: #9b5a00;
      --bad: #b42318;
      --info: #2457a6;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      font-size: 14px;
    }
    header {
      border-bottom: 1px solid var(--line);
      background: var(--panel);
    }
    .wrap {
      width: min(1180px, calc(100vw - 32px));
      margin: 0 auto;
    }
    .topbar {
      min-height: 72px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
    }
    h1 {
      margin: 0;
      font-size: 24px;
      line-height: 1.15;
      letter-spacing: 0;
    }
    .subtitle {
      margin-top: 5px;
      color: var(--muted);
      font-size: 13px;
    }
    button {
      border: 1px solid #b7c2d2;
      background: #fff;
      color: var(--text);
      height: 36px;
      padding: 0 12px;
      border-radius: 6px;
      font-weight: 650;
      cursor: pointer;
    }
    main {
      padding: 24px 0 36px;
    }
    .grid {
      display: grid;
      gap: 14px;
    }
    .cards {
      grid-template-columns: repeat(6, minmax(0, 1fr));
    }
    .card, .panel {
      background: var(--panel);
      border: 1px solid var(--line);
      border-radius: 8px;
    }
    .card {
      min-height: 104px;
      padding: 14px;
    }
    .label {
      color: var(--muted);
      font-size: 12px;
      font-weight: 700;
      text-transform: uppercase;
    }
    .value {
      margin-top: 10px;
      font-size: 28px;
      font-weight: 760;
      line-height: 1;
      letter-spacing: 0;
    }
    .hint {
      margin-top: 8px;
      color: var(--muted);
      font-size: 12px;
      min-height: 16px;
    }
    .ok { color: var(--ok); }
    .warn { color: var(--warn); }
    .bad { color: var(--bad); }
    .info { color: var(--info); }
    .layout {
      grid-template-columns: minmax(0, 1fr) 360px;
      align-items: start;
      margin-top: 14px;
    }
    .panel {
      overflow: hidden;
    }
    .panel h2 {
      margin: 0;
      padding: 15px 16px;
      border-bottom: 1px solid var(--line);
      font-size: 15px;
      letter-spacing: 0;
    }
    .panel-body {
      padding: 16px;
    }
    .bars {
      display: grid;
      gap: 12px;
    }
    .bar-row {
      display: grid;
      grid-template-columns: 120px minmax(0, 1fr) 52px;
      gap: 10px;
      align-items: center;
    }
    .bar-label, .bar-value {
      color: var(--muted);
      font-size: 12px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .bar-value {
      text-align: right;
    }
    .bar-track {
      height: 10px;
      border-radius: 999px;
      background: #edf1f6;
      overflow: hidden;
    }
    .bar-fill {
      height: 100%;
      width: 0%;
      background: var(--info);
    }
    .table-wrap {
      overflow-x: auto;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      min-width: 740px;
    }
    th, td {
      padding: 12px 14px;
      border-bottom: 1px solid var(--line);
      text-align: left;
      vertical-align: top;
    }
    th {
      color: var(--muted);
      font-size: 12px;
      font-weight: 750;
      text-transform: uppercase;
      background: #fafbfc;
    }
    td {
      font-size: 13px;
    }
    .status {
      display: inline-flex;
      align-items: center;
      min-height: 24px;
      padding: 0 8px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 750;
      background: #eef2f6;
      color: #435064;
    }
    .status.processed, .status.duplicate { background: #e6f4ee; color: var(--ok); }
    .status.failed, .status.invalid_json { background: #fdecec; color: var(--bad); }
    .status.received { background: #fff4df; color: var(--warn); }
    .chart {
      display: grid;
      grid-template-columns: repeat(24, minmax(4px, 1fr));
      gap: 4px;
      height: 150px;
      align-items: end;
      padding-top: 10px;
    }
    .column {
      min-height: 2px;
      background: var(--info);
      border-radius: 4px 4px 0 0;
    }
    .empty, .error {
      color: var(--muted);
      padding: 16px;
    }
    .error {
      color: var(--bad);
    }
    @media (max-width: 980px) {
      .cards { grid-template-columns: repeat(3, minmax(0, 1fr)); }
      .layout { grid-template-columns: 1fr; }
    }
    @media (max-width: 640px) {
      .wrap { width: min(100vw - 20px, 1180px); }
      .topbar { align-items: flex-start; flex-direction: column; padding: 14px 0; }
      .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .value { font-size: 24px; }
      .bar-row { grid-template-columns: 88px minmax(0, 1fr) 38px; }
    }
  </style>
</head>
<body>
  <header>
    <div class="wrap topbar">
      <div>
        <h1>Mentoria - Métricas</h1>
        <div class="subtitle" id="updated">Carregando dados...</div>
      </div>
      <button id="refresh" type="button">Atualizar</button>
    </div>
  </header>
  <main class="wrap">
    <section class="grid cards" id="cards"></section>
    <section class="grid layout">
      <div class="grid">
        <section class="panel">
          <h2>Eventos nas últimas 24h</h2>
          <div class="panel-body">
            <div class="chart" id="hourly"></div>
          </div>
        </section>
        <section class="panel">
          <h2>Eventos recentes</h2>
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Workflow</th>
                  <th>Status</th>
                  <th>HTTP</th>
                  <th>Criado</th>
                  <th>Erro</th>
                </tr>
              </thead>
              <tbody id="events"></tbody>
            </table>
          </div>
        </section>
      </div>
      <aside class="grid">
        <section class="panel">
          <h2>Status</h2>
          <div class="panel-body bars" id="status-bars"></div>
        </section>
        <section class="panel">
          <h2>Workflows</h2>
          <div class="panel-body bars" id="workflow-bars"></div>
        </section>
      </aside>
    </section>
  </main>
  <script>
    const cards = document.querySelector("#cards");
    const updated = document.querySelector("#updated");
    const events = document.querySelector("#events");
    const statusBars = document.querySelector("#status-bars");
    const workflowBars = document.querySelector("#workflow-bars");
    const hourly = document.querySelector("#hourly");
    const refresh = document.querySelector("#refresh");

    function metricsURL() {
      if (location.pathname.startsWith("/mentoria/")) return "/mentoria/api/metrics";
      return "/api/metrics";
    }

    function number(value) {
      return new Intl.NumberFormat("pt-BR").format(value || 0);
    }

    function percent(value) {
      return new Intl.NumberFormat("pt-BR", { style: "percent", maximumFractionDigits: 1 }).format(value || 0);
    }

    function dateTime(value) {
      if (!value) return "-";
      return new Intl.DateTimeFormat("pt-BR", {
        dateStyle: "short",
        timeStyle: "short"
      }).format(new Date(value));
    }

    function statusClass(status) {
      return String(status || "").replace(/[^a-z0-9_-]/gi, "_");
    }

    function renderCards(data) {
      const items = [
        ["Total", number(data.total_events), "Todos os eventos salvos", "info"],
        ["Hoje", number(data.events_today), "Entradas no dia atual", "info"],
        ["24h", number(data.events_last_24h), "Janela móvel", "info"],
        ["Sucesso", number(data.successful), percent(data.success_rate), "ok"],
        ["Falhas", number(data.failed), "Workflow com erro", data.failed > 0 ? "bad" : "ok"],
        ["JSON inválido", number(data.invalid_json), "Payload malformado", data.invalid_json > 0 ? "warn" : "ok"]
      ];
      cards.innerHTML = items.map(([label, value, hint, cls]) => (
        '<article class="card">' +
          '<div class="label">' + label + '</div>' +
          '<div class="value ' + cls + '">' + value + '</div>' +
          '<div class="hint">' + hint + '</div>' +
        '</article>'
      )).join("");
    }

    function renderBars(target, rows) {
      if (!rows || rows.length === 0) {
        target.innerHTML = '<div class="empty">Sem dados.</div>';
        return;
      }
      const max = Math.max(...rows.map(row => row.count), 1);
      target.innerHTML = rows.map(row => {
        const width = Math.max(4, Math.round((row.count / max) * 100));
        return '<div class="bar-row">' +
          '<div class="bar-label" title="' + row.name + '">' + row.name + '</div>' +
          '<div class="bar-track"><div class="bar-fill" style="width:' + width + '%"></div></div>' +
          '<div class="bar-value">' + number(row.count) + '</div>' +
        '</div>';
      }).join("");
    }

    function renderHourly(rows) {
      const values = rows || [];
      const max = Math.max(...values.map(row => row.count), 1);
      const padded = values.slice(-24);
      while (padded.length < 24) padded.unshift({ count: 0 });
      hourly.innerHTML = padded.map(row => {
        const height = Math.max(2, Math.round((row.count / max) * 140));
        return '<div class="column" style="height:' + height + 'px" title="' + dateTime(row.hour) + ' - ' + number(row.count) + ' eventos"></div>';
      }).join("");
    }

    function renderEvents(rows) {
      if (!rows || rows.length === 0) {
        events.innerHTML = '<tr><td colspan="6" class="empty">Nenhum evento salvo.</td></tr>';
        return;
      }
      events.innerHTML = rows.map(row => (
        '<tr>' +
          '<td>' + row.id + '</td>' +
          '<td>' + row.workflow + '</td>' +
          '<td><span class="status ' + statusClass(row.status) + '">' + row.status + '</span></td>' +
          '<td>' + (row.http_status || "-") + '</td>' +
          '<td>' + dateTime(row.created_at) + '</td>' +
          '<td>' + (row.error_message || "") + '</td>' +
        '</tr>'
      )).join("");
    }

    async function load() {
      refresh.disabled = true;
      try {
        const response = await fetch(metricsURL(), { cache: "no-store" });
        if (!response.ok) throw new Error("HTTP " + response.status);
        const data = await response.json();
        renderCards(data);
        renderBars(statusBars, data.status_counts);
        renderBars(workflowBars, data.workflow_counts);
        renderHourly(data.hourly_events);
        renderEvents(data.recent_events);
        updated.textContent = "Atualizado em " + dateTime(data.generated_at);
      } catch (error) {
        updated.textContent = "Falha ao carregar métricas";
        cards.innerHTML = '<div class="panel error">Não foi possível carregar as métricas: ' + error.message + '</div>';
      } finally {
        refresh.disabled = false;
      }
    }

    refresh.addEventListener("click", load);
    load();
  </script>
</body>
</html>`
