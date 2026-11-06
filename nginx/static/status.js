(() => {
  const message = document.getElementById('status-message');
  const indicator = document.getElementById('status-indicator');
  const updated = document.getElementById('updated-at');
  const fields = ['active', 'reading', 'writing', 'waiting', 'accepts', 'handled', 'requests'];

  function parseStatus(text) {
    const connections = text.match(/Active connections:\s*(\d+)/i);
    const totals = text.match(/server accepts handled requests\s*\n\s*(\d+)\s+(\d+)\s+(\d+)/i);
    const states = text.match(/Reading:\s*(\d+)\s+Writing:\s*(\d+)\s+Waiting:\s*(\d+)/i);
    if (!connections || !totals || !states) throw new Error('Unexpected NGINX status format');
    return { active: connections[1], reading: states[1], writing: states[2], waiting: states[3], accepts: totals[1], handled: totals[2], requests: totals[3] };
  }

  async function refresh() {
    try {
      const response = await fetch('/status/data', { cache: 'no-store' });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const status = parseStatus(await response.text());
      for (const field of fields) document.getElementById(field).textContent = Number(status[field]).toLocaleString('ru-RU');
      indicator.className = 'indicator online';
      message.textContent = 'NGINX отвечает';
      updated.textContent = `Обновлено: ${new Date().toLocaleTimeString('ru-RU')}`;
    } catch (error) {
      indicator.className = 'indicator offline';
      message.textContent = 'Не удалось получить данные NGINX';
      updated.textContent = '';
      for (const field of fields) document.getElementById(field).textContent = '—';
      console.error('NGINX status request failed:', error);
    } finally {
      setTimeout(refresh, 5000);
    }
  }
  async function refreshReadiness() {
    const message = document.getElementById('ready-message');
    const indicator = document.getElementById('ready-indicator');
    const updated = document.getElementById('ready-updated-at');
    const services = { 'user-service': 'user', 'model-service': 'model', 'inference-service': 'inference' };
    function setService(suffix, value) {
      document.getElementById('ready-' + suffix).textContent = value === 'ready' ? 'Готов' : value === 'not_ready' ? 'Не готов' : 'Неизвестно';
      document.getElementById('ready-' + suffix + '-indicator').className = 'indicator' + (value === 'ready' ? ' online' : value === 'not_ready' ? ' offline' : '');
    }
    try {
      const response = await fetch('/ready', { cache: 'no-store' });
      // 503 is a valid readiness response and contains details about failed checks.
      if (!response.ok && response.status !== 503) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      if (!data.checks || !['ready', 'not_ready'].includes(data.status)) throw new Error('Unexpected readiness format');
      for (const [name, suffix] of Object.entries(services)) setService(suffix, data.checks[name]);
      const ready = data.status === 'ready';
      indicator.className = ready ? 'indicator online' : 'indicator offline';
      message.textContent = ready ? 'Система готова' : 'Система не готова';
      updated.textContent = `Обновлено: ${new Date().toLocaleTimeString('ru-RU')}`;
    } catch (error) {
      indicator.className = 'indicator offline';
      message.textContent = 'Не удалось получить данные о готовности';
      for (const suffix of Object.values(services)) setService(suffix, undefined);
      updated.textContent = '';
      console.error('Readiness request failed:', error);
    } finally { setTimeout(refreshReadiness, 5000); }
  }
  async function refreshServices() {
    const list = document.getElementById('services-list');
    try {
      const response = await fetch('/healthz', { cache: 'no-store' });
      if (!response.ok && response.status !== 503) throw new Error(`HTTP ${response.status}`);
      const data = await response.json();
      if (!data.services || typeof data.services !== 'object') throw new Error('Unexpected health format');
      const excluded = new Set(['nginx', 'user-service', 'model-service', 'inference-service']);
      const fragment = document.createDocumentFragment();
      for (const [name, value] of Object.entries(data.services).filter(([name]) => !excluded.has(name)).sort(([a], [b]) => a.localeCompare(b))) {
        const row = document.createElement('li');
        const dot = document.createElement('span');
        const label = document.createElement('span');
        const state = document.createElement('span');
        dot.className = 'indicator' + (value === 'up' ? ' online' : value === 'down' ? ' offline' : '');
        dot.setAttribute('aria-hidden', 'true');
        label.textContent = name;
        state.className = 'readiness-state';
        state.textContent = value === 'up' ? 'Доступен' : value === 'down' ? 'Недоступен' : 'Неизвестно';
        row.append(dot, label, state);
        fragment.append(row);
      }
      list.replaceChildren(fragment);
    } catch (error) {
      const row = document.createElement('li');
      row.textContent = 'Не удалось получить статусы сервисов';
      list.replaceChildren(row);
      console.error('Service status request failed:', error);
    } finally { setTimeout(refreshServices, 5000); }
  }
  refresh();
  refreshReadiness();
  refreshServices();
})();
