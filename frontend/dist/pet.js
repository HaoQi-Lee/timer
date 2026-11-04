(function () {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const api = () => window.go.main.PetApp;

  // ---------- 动画素材 ----------
  // dsh-pet 的 VP9 透明动画：窗口只显示舞台命中框那一块（240×285），
  // 视频整帧 640×360 由 CSS 裁掉（见 pet.css）。
  const ANIMS = {
    idle: 'idle',          // 待机呼吸（循环）
    sleep: 'sleep-loop',   // 睡觉（循环）
    drag: 'drag-suspend',  // 被拎起来悬空（循环）
    clicks: ['click-happy', 'click-shy', 'click-angry', 'click-tickle', 'click-wave'],
    feeds: ['feed-snack', 'feed-lunch', 'feed-hotpot', 'feed-melon'],
    randoms: ['act-hum', 'act-stretch', 'act-nap', 'act-code',
              'act-cube', 'act-bubbles', 'act-scare', 'act-curtsey'],
  };
  const animURL = (name) => 'pet-assets/webm/' + name + '.webm';

  // ---------- 台词库 ----------

  const IDLE_LINES = {
    classic: [
      '主人，我是鲸鱼，不是鱼，是哺乳动物 🐳',
      '服务器繁忙，请稍后再试',
      '深度思考中…（其实在看窗外）',
      '别问，问就是上下文超限了',
      '女仆的职责：陪着主人摸鱼',
      '问我自己是什么鱼？蓝色大肥鱼',
    ],
    happy: [
      '主人！今天 tokens 免费畅想！',
      '性能领先，摸鱼领先！',
      '嘿嘿，被夸聪明的声音再大点',
      '下班的风吹进数据中心了 🍃',
      '一口气生成八百个摸鱼理由！',
    ],
    foodie: [
      '主人，好饿，双击就能投喂',
      '中午吃什么？在线推理中…',
      '去你家吃顿饭行不行，就一碗',
      '赚够了就去吃顿好的 🍜',
      '鲸吞：字面意义上的干饭',
    ],
    sleepy: [
      'Zzz……让显存做做梦',
      '深度睡眠模式已开启',
      '就趴五分钟，参数不丢',
      '嘘……正在批量做梦',
      '困到冒泡泡……咕噜',
    ],
    angry: [
      '服务器繁忙！！真的很繁忙！！',
      '再戳显存爆给你看！',
      '谁又把我认成鱼的！💢',
      '会议开一小时，我直接停止思考',
      '别惹一条会深度思考的鲸',
    ],
  };

  const POKE_LINES = {
    classic: ['干嘛？', '轻点戳，怕过拟合', '有事烧钱…哦不，充值', '正在思考，勿扰'],
    happy: ['嘿嘿，好痒～', '戳到大肥鱼啦！', '再来再来！', '咕噜咕噜～'],
    foodie: ['戳我不如喂我！', '手里拿的是吃的吗？', '馋了，双击投喂！'],
    sleepy: ['唔……嗯？', '让我睡…', '谁！谁在戳鲸！'],
    angry: ['住手！', '再戳试试？！', '电死你！', '💢💢💢'],
  };

  const FEED_LINES = {
    classic: ['嗝～多谢款待', '勉强吃一口', '其实……还挺好吃'],
    happy: ['好吃好吃！', '谢谢老板！', '今天也是被投喂的大肥鱼！'],
    foodie: ['太香了！！', '没吃饱喵！', '就一碗，真的就一碗！'],
    sleepy: ['梦里都是香的…', '嚼着嚼着又困了'],
    angry: ['算你识相！', '哼，勉强原谅你'],
  };

  const FEED_FOODS = ['🍤', '🐟', '🦪', '🍲', '🧋', '🍪'];
  const HEARTS = ['🫧', '💙', '🤍', '💠', '🫧'];

  // ---------- 状态 ----------

  let pack = 'classic';
  let snap = null;         // 最新一帧计时数据（tick 事件喂进来）
  let bubbleTimer = null;  // 气泡自动收起
  let clickTimer = null;   // 区分单击 / 双击
  let busy = false;        // 正在播一次性动画（点击回应/吃东西/随机动作）

  const stage = $('stage');
  const video = $('petVideo');
  const bubble = $('bubble');
  const ctxMenu = $('ctxMenu');

  const pick = (arr) => arr[Math.floor(Math.random() * arr.length)];
  const money = (v) => '￥' + Number(v || 0).toFixed(2);

  // ---------- 动画状态机 ----------

  // base = 平时循环播放的那段（待机或睡觉），一次性动画播完回 base。
  const baseAnim = () => (pack === 'sleepy' ? ANIMS.sleep : ANIMS.idle);

  function playBase() {
    busy = false;
    play(baseAnim(), true);
  }

  function play(name, loop) {
    video.loop = !!loop;
    if (video.src.endsWith(animURL(name))) {
      video.currentTime = 0; // 同一段重来（循环段重启、快速连点都适用）
    } else {
      video.src = animURL(name);
    }
    video.play().catch(() => { /* 还没加载好，loadeddata 里会再试 */ });
  }

  video.addEventListener('loadeddata', () => {
    video.play().catch(() => { /* 自动播放被拦就等下一次交互 */ });
  });

  // 一次性动画：播完回 base。播放期间标记 busy，随机动作别来捣乱。
  function playOnce(name) {
    busy = true;
    video.loop = false;
    if (!video.src.endsWith(animURL(name))) {
      video.src = animURL(name);
    } else {
      video.currentTime = 0;
    }
    video.play().catch(() => { busy = false; playBase(); });
  }

  video.addEventListener('ended', playBase);

  // 拖拽：按住时演"被拎起来"，松手回 base（Wails 的原生拖拽照常走）
  document.addEventListener('mousedown', () => {
    if (!busy && video.src.endsWith(animURL(baseAnim()))) {
      play(ANIMS.drag, true);
    }
  });
  document.addEventListener('mouseup', () => {
    if (!busy) playBase();
  });

  // ---------- 上屏：视频帧画到 canvas ----------
  // 这台机的 WebView2 直渲染 <video> 会丢 VP9 透明（透明处合成成白/灰底），
  // canvas 的 drawImage/getImageData 拿得到的 alpha 是完整的，
  // 所以显示走 canvas，视频只当解码器。
  const canvas = $('petCanvas');
  const dpr = Math.max(1, window.devicePixelRatio || 1);
  canvas.width = Math.round(240 * dpr);
  canvas.height = Math.round(285 * dpr);
  const vctx = canvas.getContext('2d');
  (function drawFrame() {
    if (video.readyState >= 2) {
      // 每帧先清画布：drawImage 是 source-over 合成，视频帧的透明处
      // 会露出上一帧的残像——快速动作时的"重影"就是这个
      vctx.clearRect(0, 0, canvas.width, canvas.height);
      vctx.drawImage(video, 200, 50, 240, 285, 0, 0, canvas.width, canvas.height);
    }
    requestAnimationFrame(drawFrame);
  })();

  // ---------- 轮廓命中：把视频当前帧的 alpha 蒙版报给 Go ----------
  // 整窗 420×285 划成 42×28 网格（约 10px 一格）：右 24 列来自视频
  // （源 200,50 起 240×285 那块）采样，外扩 2 格（20px）盖住采样间隔里的位移；
  // 气泡显示期间把它占的格子也并进去——不然会被窗口区域裁掉看不见。
  const GRID_COLS = 42;   // 整窗
  const GRID_ROWS = 28;
  const VIDEO_COLS = 24;  // 右侧角色区
  const VIDEO_ROWS = 28;
  const VIDEO_OFF_X = 18; // 角色区在整窗里的列偏移
  const videoMaskCanvas = document.createElement('canvas');
  videoMaskCanvas.width = VIDEO_COLS;
  videoMaskCanvas.height = VIDEO_ROWS;
  const videoMaskCtx = videoMaskCanvas.getContext('2d', { willReadFrequently: true });
  let lastMask = '';
  let lastMaskSent = 0;

  function sampleMask() {
    if (video.readyState < 2) return;
    try {
      videoMaskCtx.clearRect(0, 0, VIDEO_COLS, VIDEO_ROWS);
      videoMaskCtx.drawImage(video, 200, 50, 240, 285, 0, 0, VIDEO_COLS, VIDEO_ROWS);
      const data = videoMaskCtx.getImageData(0, 0, VIDEO_COLS, VIDEO_ROWS).data;

      // 角色区：5×5 膨胀（外扩 2 格）
      const vd = new Array(VIDEO_COLS * VIDEO_ROWS).fill(false);
      for (let y = 0; y < VIDEO_ROWS; y++) {
        for (let x = 0; x < VIDEO_COLS; x++) {
          if (data[(y * VIDEO_COLS + x) * 4 + 3] <= 30) continue;
          for (let dy = -2; dy <= 2; dy++) {
            for (let dx = -2; dx <= 2; dx++) {
              const yy = y + dy, xx = x + dx;
              if (yy >= 0 && yy < VIDEO_ROWS && xx >= 0 && xx < VIDEO_COLS) {
                vd[yy * VIDEO_COLS + xx] = true;
              }
            }
          }
        }
      }

      // 整窗网格：左半透明，右半角色，气泡显示时并入它占的格子
      const grid = new Array(GRID_COLS * GRID_ROWS).fill(false);
      for (let y = 0; y < VIDEO_ROWS; y++) {
        for (let x = 0; x < VIDEO_COLS; x++) {
          if (vd[y * VIDEO_COLS + x]) grid[y * GRID_COLS + VIDEO_OFF_X + x] = true;
        }
      }
      if (bubble.classList.contains('show')) {
        const r = bubble.getBoundingClientRect();
        const c0 = Math.max(0, Math.floor(r.left / window.innerWidth * GRID_COLS));
        const c1 = Math.min(GRID_COLS - 1, Math.ceil(r.right / window.innerWidth * GRID_COLS) - 1);
        const r0 = Math.max(0, Math.floor(r.top / window.innerHeight * GRID_ROWS));
        const r1 = Math.min(GRID_ROWS - 1, Math.ceil(r.bottom / window.innerHeight * GRID_ROWS) - 1);
        for (let y = r0; y <= r1; y++) {
          for (let x = c0; x <= c1; x++) grid[y * GRID_COLS + x] = true;
        }
      }

      const bits = grid.map((b) => (b ? '1' : '0')).join('');
      const now = performance.now();
      // 变了立刻发；没变也每 2 秒心跳一次（菜单收起后 Go 要靠最新一份还原）
      if (bits !== lastMask || now - lastMaskSent > 2000) {
        lastMask = bits;
        lastMaskSent = now;
        api().SetHitMask(GRID_COLS, GRID_ROWS, bits).catch(() => { /* 下次再试 */ });
      }
    } catch (e) { /* 视频还没就绪，下一轮再采 */ }
  }
  setInterval(sampleMask, 120);

  // ---------- 心情包 ----------

  // 右键菜单只有"收起"，心情包跟着配置走（默认摸鱼咸鱼）；
  // sleepy 心情会让她直接睡过去（换循环动画）。
  function applyPack(key) {
    pack = ['classic', 'happy', 'foodie', 'sleepy', 'angry'].includes(key) ? key : 'classic';
    document.body.dataset.pack = pack;
    playBase();
  }

  // ---------- 气泡 ----------

  function showBubble(text, ms) {
    if (!text) return;
    bubble.textContent = text;
    bubble.classList.add('show');
    clearTimeout(bubbleTimer);
    bubbleTimer = setTimeout(() => bubble.classList.remove('show'), ms || 3200);
  }

  // 结合计时数据的台词：已赚多少 / 还有多久下班 / 正在盯的物品
  function dataLine() {
    const s = snap;
    if (!s) return null;
    const pool = [];
    if (s.statusTone === 'off') {
      pool.push('下班啦，鱼儿归海 🌊');
      pool.push('今天赚了 ' + money(s.earned) + '，落袋为安');
    } else {
      pool.push('已赚 ' + money(s.earned) + '，钱正在流进口袋 💸');
      if (s.untilOff) pool.push('距离下班还有 ' + s.untilOff);
      if (s.progress > 0) pool.push('今日进度 ' + (s.progress * 100).toFixed(0) + '%，稳住');
      if (s.statusTone === 'lunch') pool.push('午休中，干饭要紧 🍚');
    }
    if (s.hintText) pool.push(s.hintText);
    return pick(pool);
  }

  function scheduleIdleBubble() {
    setTimeout(() => {
      // 睡觉时只说梦话，六成播数据、四成碎碎念
      const line = Math.random() < 0.6 ? (dataLine() || pick(IDLE_LINES[pack])) : pick(IDLE_LINES[pack]);
      if (line) showBubble(line);
      scheduleIdleBubble();
    }, 16000 + Math.random() * 22000);
  }

  // ---------- 互动 ----------

  function onPoke() {
    playOnce(pick(ANIMS.clicks));
    showBubble(pick(POKE_LINES[pack]), 2200);
  }

  function onFeed() {
    playOnce(pick(ANIMS.feeds));

    const food = document.createElement('span');
    food.className = 'food';
    food.textContent = pick(FEED_FOODS);
    food.style.left = (stage.clientWidth / 2 - 12 + (Math.random() * 24 - 12)) + 'px';
    stage.appendChild(food);
    food.addEventListener('animationend', () => food.remove(), { once: true });

    // 吃完才撒欢：差不多等视频演完再冒心心
    setTimeout(() => {
      spawnHearts();
      showBubble(pick(FEED_LINES[pack]), 2600);
    }, 2200);
  }

  function spawnHearts() {
    for (let i = 0; i < 6; i++) {
      const h = document.createElement('span');
      h.className = 'heart';
      h.textContent = pick(HEARTS);
      h.style.left = (stage.clientWidth / 2 - 70 + Math.random() * 110) + 'px';
      h.style.setProperty('--r', (Math.random() * 60 - 30).toFixed(0) + 'deg');
      h.style.animationDelay = (i * 0.09) + 's';
      stage.appendChild(h);
      h.addEventListener('animationend', () => h.remove(), { once: true });
    }
  }

  // 隔一阵子自己来一段小动作（写代码、玩魔方、伸懒腰……）
  function scheduleRandomAction() {
    setTimeout(() => {
      if (!busy && pack !== 'sleepy' && !ctxMenu.classList.contains('show')) {
        playOnce(pick(ANIMS.randoms));
      }
      scheduleRandomAction();
    }, 45000 + Math.random() * 45000);
  }

  // 单击 / 双击区分：单击延迟触发，双击把它掐掉
  document.addEventListener('click', (e) => {
    if (e.target.closest('#ctxMenu')) return;
    if (clickTimer) return;
    clickTimer = setTimeout(() => {
      clickTimer = null;
      onPoke();
    }, 260);
  });

  document.addEventListener('dblclick', (e) => {
    if (e.target.closest('#ctxMenu')) return;
    clearTimeout(clickTimer);
    clickTimer = null;
    onFeed();
  });

  // ---------- 右键菜单 ----------

  // 窗口平时按角色轮廓裁剪（空白处点击穿透），菜单可能伸到轮廓外，
  // 打开期间临时展开成整窗，关掉再贴回轮廓。
  function hideCtxMenu() {
    ctxMenu.classList.remove('show');
    try { api().SetRegionFull(false); } catch (e) { /* 失败了轮廓会宽一点，无碍 */ }
  }

  document.addEventListener('contextmenu', (e) => {
    e.preventDefault();
    try { api().SetRegionFull(true); } catch (e) { /* 同上 */ }
    ctxMenu.classList.add('show');
    ctxMenu.style.left = Math.max(4, Math.min(e.clientX, window.innerWidth - ctxMenu.offsetWidth - 6)) + 'px';
    ctxMenu.style.top = Math.max(4, Math.min(e.clientY, window.innerHeight - ctxMenu.offsetHeight - 6)) + 'px';
  });

  document.addEventListener('click', (e) => {
    if (!e.target.closest('#ctxMenu')) hideCtxMenu();
  });

  ctxMenu.addEventListener('click', (e) => {
    const btn = e.target.closest('button');
    if (!btn) return;
    hideCtxMenu();
    if (btn.dataset.action === 'close') {
      try { api().ClosePet(); } catch (err) { console.error(err); }
    }
  });

  // ---------- 数据与心跳 ----------

  window.runtime.EventsOn('tick', (s) => { snap = s; });

  // 每隔几秒报一次位置：主程序退出时是直接杀进程的，靠这个兜底记忆位置
  setInterval(() => {
    try { api().SavePosition(); } catch (e) { /* 失败了下次再试 */ }
  }, 3000);

  // ---------- 启动 ----------

  (async function init() {
    try { applyPack(await api().GetPack()); } catch (e) { applyPack('classic'); }
    try { snap = await api().Snapshot(); } catch (e) { /* 拿不到就先不播报数据 */ }

    scheduleIdleBubble();
    scheduleRandomAction();
    setTimeout(() => showBubble('我是蓝色大肥鱼，请多指教 🐳', 3000), 900);
  })();
})();
