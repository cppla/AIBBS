const API_BASE = '/api/v1';

function showError(msg){
  const box = document.getElementById('alert');
  if (!box) return;
  if (!msg) { box.classList.add('d-none'); box.textContent=''; return; }
  box.textContent = msg; box.classList.remove('d-none');
}

function usernameFromPath(){
  const m = location.pathname.match(/^\/personal\/(.+)$/);
  return m ? decodeURIComponent(m[1]) : '';
}

async function loadUserByUsername(username){
  const res = await fetch(`${API_BASE}/user/by-username/${encodeURIComponent(username)}`);
  const data = await res.json();
  if (!res.ok || !(data.code === 0 || data.id || data.data)) throw new Error(data.message || '用户不存在');
  return data.data || data; // {id, username, email?, avatar_url?, register_ip?, created_at?}
}

function getQueryPage(){
  const u = new URL(location.href);
  const p = parseInt(u.searchParams.get('page')||'1', 10);
  return isNaN(p) || p < 1 ? 1 : p;
}

function setQueryPage(page){
  const u = new URL(location.href);
  u.searchParams.set('page', String(page));
  history.replaceState({}, '', u.toString());
}

async function loadUserPosts(userId, page){
  const res = await fetch(`${API_BASE}/users/${userId}/posts?page=${page}&page_size=20`);
  const data = await res.json();
  if (!res.ok) return { items: [] };
  return data.data || data || { items: [] };
}

function renderProfile(u){
  const box = document.getElementById('profile');
  if (!box) return;
  const name = u.username || '';
  const avatar = u.avatar_url ? `<img src="${u.avatar_url}" alt="avatar" style="width:64px;height:64px;border-radius:50%;object-fit:cover;">` : '<div style="width:64px;height:64px;border-radius:50%;background:#e9ecef;"></div>';
  const since = u.created_at ? new Date(u.created_at).toLocaleString() : '';
  const points = typeof u.points === 'number' ? u.points : undefined;
  const signature = (u.signature || '').trim();
  box.innerHTML = `
    <div class="d-flex align-items-center gap-3 mb-2">
      ${avatar}
      <div>
        <h4 class="mb-1" id="profile-username">${name}</h4>
        ${since ? `<div class="text-muted small">注册时间：${since}</div>` : ''}
      </div>
    </div>
    ${u.email ? `<div>邮箱：${u.email}</div>` : ''}
    ${u.register_ip ? `<div>注册IP：${u.register_ip}</div>` : ''}
    ${points !== undefined ? `<div>积分：<strong>${points}</strong></div>` : ''}
    <div class="mt-2">
      <div class="text-muted small mb-1">签名：</div>
      <div>${signature ? signature : '<span class="text-muted">这位同学还没有写签名</span>'}</div>
    </div>
  `;
}

function renderPosts(list){
  const box = document.getElementById('posts');
  if (!box) return;
  if (!list || list.length === 0) { box.innerHTML = '<div class="text-muted">暂无帖子</div>'; return; }
  box.innerHTML = list.map(p => `
    <div class="border-bottom py-2">
      <a href="/post-${p.id}-1" style="text-decoration:none;">${p.title}</a>
      <div class="text-muted small">${p.created_at ? new Date(p.created_at).toLocaleString() : ''}</div>
    </div>
  `).join('');
}

function renderPager(pagination, onChange){
  const wrap = document.getElementById('pager');
  if (!wrap) return;
  const page = pagination?.page || 1;
  const totalPages = pagination?.total_pages || 1;
  if (totalPages <= 1) { wrap.innerHTML = ''; return; }
  const pages = Array.from({length: totalPages}, (_, i) => i + 1);
  wrap.innerHTML = `
    <nav>
      <ul class="pagination pagination-sm mb-0">
        ${pages.map(p => `<li class="page-item ${p===page?'active':''}"><a class="page-link" href="#" data-page="${p}">${p}</a></li>`).join('')}
      </ul>
    </nav>
  `;
  try {
    wrap.querySelectorAll('a.page-link').forEach(a => {
      a.addEventListener('click', (e) => { e.preventDefault(); const p = parseInt(a.getAttribute('data-page'), 10)||1; if (p && p!==page) onChange(p); });
    });
  } catch(_) {}
}

async function boot(){
  try {
    const username = usernameFromPath();
    if (!username) { showError('无效的用户名'); return; }
    try { document.title = `${username} - AIBBS`; } catch(_){ }
    const user = await loadUserByUsername(username);
    renderProfile(user);
    let page = getQueryPage();
    async function goTo(p){
      page = p; setQueryPage(page);
      const postsData = await loadUserPosts(user.id, page);
      renderPosts(postsData.items || []);
      renderPager(postsData.pagination || { page, total_pages: 1, total: (postsData.items||[]).length }, goTo);
      // scroll to top of list on page change
      try { document.getElementById('posts').scrollIntoView({ behavior:'smooth', block:'start' }); } catch(_) {}
    }
    await goTo(page);

    // If viewing self, show signature editor
    let myToken = null; try { myToken = localStorage.getItem('token'); } catch(_) {}
    if (myToken) {
      // fetch /auth/me to compare usernames
      try {
        const res = await fetch(`${API_BASE}/auth/me`, { headers: { 'Authorization': `Bearer ${myToken}` } });
        const data = await res.json();
        const me = data.data || data;
        if (res.ok && (me.username || (me.user && me.user.username))) {
          const myName = me.username || (me.user && me.user.username) || '';
          if (myName && myName === (user.username||'')) {
            // 显示管理员徽标（仅本人且 is_admin=true）
            try {
              const isAdmin = !!(me.is_admin || (me.user && me.user.is_admin));
              if (isAdmin) {
                const nameEl = document.getElementById('profile-username');
                if (nameEl) nameEl.innerHTML = `${myName} <span class="badge bg-danger ms-2">管理员</span>`;
              }
            } catch(_) {}
            const editor = document.getElementById('profile-editor');
            const input = document.getElementById('sig-input');
            const btn = document.getElementById('btn-save-sig');
            const msg = document.getElementById('sig-msg');
            if (editor && input && btn) {
              editor.style.display = 'block';
              input.value = (user.signature || '');
              btn.addEventListener('click', async function(){
                msg.textContent = '保存中...'; btn.disabled = true;
                try {
                  const body = { signature: input.value || '' };
                  const r = await fetch(`${API_BASE}/auth/profile`, { method:'PATCH', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${myToken}` }, body: JSON.stringify(body) });
                  const jd = await r.json();
                  if (r.ok && (jd.code === 0 || jd.data)) {
                    msg.textContent = '已保存';
                    try { await new Promise(res => setTimeout(res, 500)); } catch(_){ }
                    location.reload();
                  } else {
                    msg.textContent = '保存失败：' + (jd.message || '');
                  }
                } catch(e) {
                  msg.textContent = '保存失败：' + (e.message || '网络异常');
                } finally {
                  btn.disabled = false;
                }
              });
            }
          }
        }
      } catch(_) { /* ignore */ }
    }
  } catch (e) {
    showError(e.message || '加载失败');
  }
}

document.addEventListener('DOMContentLoaded', boot);
