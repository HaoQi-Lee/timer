(function () {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const api = () => window.go.main.App;

  const WIDGET_SIZE = [340, 500];
  const PANEL_SIZE = [460, 660];
  const WORKDAYS = 261;

  let settings = null;
  let customGoods = [];
  let topOn = true;
  let petOn = false;
  let theme = 'dark';

  // ---------- 主题 ----------

  function applyTheme(t) {
    theme = t === 'light' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    $('btnTheme').textContent = theme === 'light' ? '☀️' : '🌙';
    $('btnTheme').title = theme === 'light' ? '切换到深色主题' : '切换到浅色主题';
    $('lightTheme').checked = theme === 'light';
    try {
      localStorage.setItem('work-timer-theme', theme);
    } catch (e) {
      // localStorage 不可用也不影响使用
    }
  }

  async function setTheme(t) {
    applyTheme(t);
    try {
      await api().SetTheme(theme);
    } catch (e) {
      console.error('保存主题失败', e);
    }
  }

  // ---------- 渲染 ----------

  function money(v, digits) {
    return '￥' + Number(v || 0).toFixed(digits === undefined ? 2 : digits);
  }

  function render(s) {
    document.documentElement.dataset.tone = s.statusTone;

    $('statusText').textContent = s.status;
    $('clock').textContent = s.clock;
    $('dateLine').textContent = s.date + ' ' + s.weekday;

    if (s.statusTone === 'off') {
      $('countdownLabel').textContent = '今日已下班';
      $('untilOff').textContent = '辛苦啦，明天见';
    } else {
      $('countdownLabel').textContent = '距离下班还有';
      $('untilOff').textContent = s.untilOff;
    }
    $('untilStart').textContent = s.untilStart || '';

    $('workedText').textContent = s.worked + ' / ' + s.totalWork;
    $('progressFill').style.width = (s.progress * 100).toFixed(2) + '%';

    $('earned').textContent = money(s.earned);
    $('rate').textContent = money(s.hourlyRate, 1) + ' /小时 · ' + money(s.minuteRate, 2) + ' /分钟';

    $('hintEmoji').textContent = s.hintEmoji || '💡';
    $('hintText').textContent = s.hintText;

    // 有具体物品时才显示进度条；没有物品（比如还没设年薪）就只留一句话。
    const hasItem = !!s.hintItem;
    $('hintProgress').style.display = hasItem ? 'flex' : 'none';
    if (hasItem) {
      const p = Math.max(0, Math.min(1, Number(s.hintProgress) || 0));
      $('hintFill').style.width = (p * 100).toFixed(2) + '%';
      $('hintAmount').textContent = money(s.earned, 1) + ' / ' + money(s.hintPrice, 1);
    }
  }

  // ---------- 设置表单 ----------

  function fillForm(s) {
    $('annualSalary').value = s.annualSalary;
    $('workStart').value = s.workStart;
    $('lunchStart').value = s.lunchStart;
    $('workResume').value = s.workResume;
    $('workEnd').value = s.workEnd;
    $('alwaysOnTop').checked = !!s.alwaysOnTop;
    topOn = !!s.alwaysOnTop;
    updateTopLabel();
    $('petEnabled').checked = !!s.petEnabled;
    petOn = !!s.petEnabled;
    updatePetLabel();
    applyTheme(s.theme);
    customGoods = (s.customGoods || []).slice();
    renderGoods();
    updateSalaryHint();
  }

  function updateSalaryHint() {
    const annual = Number($('annualSalary').value) || 0;
    $('salaryHint').textContent = annual > 0
      ? '按每年 ' + WORKDAYS + ' 个法定工作日折算，日薪 ≈ ' + money(annual / WORKDAYS)
      : '按每年 ' + WORKDAYS + ' 个法定工作日折算日薪，填入年薪后这里会显示日薪。';
  }

  function renderGoods() {
    const box = $('goodsList');
    box.innerHTML = '';

    if (!customGoods.length) {
      const p = document.createElement('p');
      p.className = 'empty';
      p.textContent = '还没有自定义物品';
      box.appendChild(p);
      return;
    }

    customGoods.forEach((g, i) => {
      const row = document.createElement('div');
      row.className = 'goods-row';

      const name = document.createElement('span');
      name.className = 'goods-name';
      name.textContent = (g.emoji || '🛍️') + ' ' + g.name;

      const price = document.createElement('span');
      price.className = 'goods-price';
      price.textContent = '￥' + Number(g.price || 0).toFixed(0);

      const del = document.createElement('button');
      del.className = 'goods-del';
      del.textContent = '✕';
      del.title = '删除';
      del.addEventListener('click', () => {
        customGoods.splice(i, 1);
        renderGoods();
      });

      row.appendChild(name);
      row.appendChild(price);
      row.appendChild(del);
      box.appendChild(row);
    });
  }

  function addGoods() {
    const name = $('newGoodsName').value.trim();
    const price = Number($('newGoodsPrice').value);
    if (!name) { $('newGoodsName').focus(); return; }
    if (!(price > 0)) { $('newGoodsPrice').focus(); return; }
    customGoods.push({ name: name, price: price, emoji: '🛍️' });
    $('newGoodsName').value = '';
    $('newGoodsPrice').value = '';
    renderGoods();
  }

  // ---------- 面板开合 ----------

  async function openSettings() {
    try {
      settings = await api().GetSettings();
      fillForm(settings);
    } catch (e) {
      console.error('读取设置失败', e);
    }
    $('overlay').classList.add('show');
    setSize(PANEL_SIZE);
  }

  function closeSettings() {
    $('overlay').classList.remove('show');
    setSize(WIDGET_SIZE);
  }

  function setSize(size) {
    try {
      window.runtime.WindowSetSize(size[0], size[1]);
    } catch (e) {
      console.error('调整窗口大小失败', e);
    }
  }

  async function saveSettings() {
    const payload = {
      annualSalary: Number($('annualSalary').value) || 0,
      workStart: $('workStart').value || '08:30',
      lunchStart: $('lunchStart').value || '11:30',
      workResume: $('workResume').value || '13:00',
      workEnd: $('workEnd').value || '17:00',
      alwaysOnTop: $('alwaysOnTop').checked,
      theme: theme,
      customGoods: customGoods,
      windowX: settings ? (settings.windowX || 0) : 0,
      windowY: settings ? (settings.windowY || 0) : 0,
      petEnabled: $('petEnabled').checked,
      petPack: settings ? (settings.petPack || 'classic') : 'classic',
      petX: settings ? (settings.petX || 0) : 0,
      petY: settings ? (settings.petY || 0) : 0
    };
    try {
      await api().SaveSettings(payload);
      await api().SetAlwaysOnTop(payload.alwaysOnTop);
      await api().SetPetEnabled(payload.petEnabled);
      petOn = payload.petEnabled;
      settings = payload;
      closeSettings();
    } catch (e) {
      window.alert('保存失败：' + e);
    }
  }

  function updateTopLabel() {
    $('ctxToggleTop').textContent = topOn ? '取消置顶' : '始终置顶';
  }

  async function toggleTop() {
    topOn = !topOn;
    try {
      await api().SetAlwaysOnTop(topOn);
    } catch (e) {
      console.error('切换置顶失败', e);
    }
    $('alwaysOnTop').checked = topOn;
    updateTopLabel();
  }

  // ---------- 桌面宠物 ----------

  function updatePetLabel() {
    $('ctxTogglePet').textContent = petOn ? '收起桌面宠物' : '开启桌面宠物';
  }

  async function togglePet() {
    const next = !petOn;
    try {
      await api().SetPetEnabled(next);
      petOn = next;
    } catch (e) {
      console.error('切换桌面宠物失败', e);
    }
    $('petEnabled').checked = petOn;
    updatePetLabel();
  }

  // ---------- 右键菜单 ----------

  function hideCtxMenu() {
    $('ctxMenu').classList.remove('show');
  }

  document.addEventListener('contextmenu', (e) => {
    if ($('overlay').classList.contains('show')) return;
    // 输入框里保留系统自带的复制/粘贴菜单
    const tag = e.target && e.target.tagName;
    if (tag === 'INPUT' || tag === 'TEXTAREA') return;
    e.preventDefault();
    const m = $('ctxMenu');
    m.classList.add('show');
    m.style.left = Math.max(4, Math.min(e.clientX, window.innerWidth - m.offsetWidth - 6)) + 'px';
    m.style.top = Math.max(4, Math.min(e.clientY, window.innerHeight - m.offsetHeight - 6)) + 'px';
  });

  document.addEventListener('click', (e) => {
    if (!e.target.closest('#ctxMenu')) hideCtxMenu();
  });

  $('ctxMenu').addEventListener('click', (e) => {
    const btn = e.target.closest('button[data-action]');
    if (!btn) return;
    hideCtxMenu();
    switch (btn.dataset.action) {
      case 'settings': openSettings(); break;
      case 'toggle-top': toggleTop(); break;
      case 'toggle-pet': togglePet(); break;
      case 'quit': api().Quit(); break;
    }
  });

  // ---------- 事件绑定 ----------

  $('btnSettings').addEventListener('click', openSettings);
  $('btnClose').addEventListener('click', () => api().Quit());
  $('btnMin').addEventListener('click', () => api().Minimize());
  $('btnTheme').addEventListener('click', () => setTheme(theme === 'light' ? 'dark' : 'light'));
  $('btnCloseSettings').addEventListener('click', closeSettings);
  $('btnCancelSettings').addEventListener('click', closeSettings);
  $('btnSaveSettings').addEventListener('click', saveSettings);
  $('btnAddGoods').addEventListener('click', addGoods);
  $('hintBox').addEventListener('click', openSettings);
  $('annualSalary').addEventListener('input', updateSalaryHint);
  $('lightTheme').addEventListener('change', (e) => setTheme(e.target.checked ? 'light' : 'dark'));
  $('newGoodsPrice').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') addGoods();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') closeSettings();
  });

  window.runtime.EventsOn('tick', render);
  window.runtime.EventsOn('open-settings', openSettings);
  // 宠物自己被收起时，主窗口这边同步开关状态
  window.runtime.EventsOn('pet-state', (on) => {
    petOn = !!on;
    $('petEnabled').checked = petOn;
    updatePetLabel();
  });

  // ---------- 启动 ----------

  (async function init() {
    try {
      const s = await api().Snapshot();
      render(s);
      settings = await api().GetSettings();
      fillForm(settings);
      if (s.openSettings) openSettings();
    } catch (e) {
      console.error('初始化失败', e);
    }
  })();
})();
