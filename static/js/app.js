// 使用相对路径，避免跨域/端口不一致导致请求失败
const API_BASE = '/api/v1';
const BASE_TITLE = 'AIBBS';
const HOME_TAGLINE = ' 全球第一个AI驱动的论坛系统';
let currentUser = null;
let currentPage = 1;
let searchQuery = '';
let currentCategory = '';
let isAdminView = false;
let currentCommentsPage = 1;
const COMMENTS_PAGE_SIZE = 10;
let currentListContext = { type: 'home' }; // {type:'home'|'category'|'user', userId?:number}

// In-app forum-style notifications
function ensureNotifyHost() {
    let host = document.getElementById('notify-container');
    if (!host) {
        host = document.createElement('div');
        host.id = 'notify-container';
        host.style.position = 'fixed';
        host.style.top = '1rem';
        host.style.right = '1rem';
        host.style.zIndex = '1080';
        host.style.width = 'min(420px, 90vw)';
        document.body.appendChild(host);
    }
    return host;
}

function notify(message, type = 'info', timeoutMs = 3000) {
    try {
        const host = ensureNotifyHost();
        const div = document.createElement('div');
        const cls = type === 'success' ? 'alert-success' : type === 'error' ? 'alert-danger' : type === 'warning' ? 'alert-warning' : 'alert-info';
        div.className = `alert ${cls} shadow-sm border-0`;
        div.role = 'alert';
        div.style.marginBottom = '.5rem';
        div.innerHTML = `<div class="d-flex align-items-start"><div class="flex-grow-1">${message}</div><button type="button" class="btn-close ms-2" aria-label="Close"></button></div>`;
        const closeBtn = div.querySelector('.btn-close');
        closeBtn.addEventListener('click', () => { try { host.removeChild(div); } catch(_) {} });
        host.appendChild(div);
        if (timeoutMs > 0) setTimeout(() => { try { host.removeChild(div); } catch(_) {} }, timeoutMs);
    } catch (_) {
        console[type === 'error' ? 'error' : 'log'](`[notify:${type}]`, message);
    }
}

// Upload selected file to local server which stores into /static/uploads/YYYY/MM/DD and returns {url}
async function uploadFileToLocal(file) {
    const fd = new FormData();
    fd.append('file', file, file.name || 'file');
    const res = await fetch(`${API_BASE}/upload`, { method: 'POST', headers: { 'Authorization': `Bearer ${getToken()}` }, body: fd });
    const data = await res.json();
    if (!res.ok || (data && data.code && data.code !== 0)) {
        const msg = data?.message || `HTTP ${res.status}`;
        throw new Error(msg);
    }
    const url = data?.data?.url || data?.url;
    if (!url) throw new Error('上传结果缺少URL');
    return url;
}

// Insert markdown image or plain URL into editor
function insertUrlIntoEditor(editorOrTextarea, url, alt = '') {
    try {
        if (editorOrTextarea && editorOrTextarea.codemirror) {
            const cm = editorOrTextarea.codemirror;
            const md = url.match(/\.(png|jpe?g|gif|webp|svg)(\?|#|$)/i) ? `![${alt || 'image'}](${url})` : url;
            cm.replaceSelection(md + '\n');
            cm.focus();
        } else if (editorOrTextarea && typeof editorOrTextarea.value === 'function') {
            const cur = editorOrTextarea.value();
            const md = url.match(/\.(png|jpe?g|gif|webp|svg)(\?|#|$)/i) ? `![${alt || 'image'}](${url})` : url;
            editorOrTextarea.value((cur ? (cur + '\n') : '') + md + '\n');
        } else if (editorOrTextarea && editorOrTextarea.tagName === 'TEXTAREA') {
            const md = url.match(/\.(png|jpe?g|gif|webp|svg)(\?|#|$)/i) ? `![${alt || 'image'}](${url})` : url;
            editorOrTextarea.value = (editorOrTextarea.value || '') + (editorOrTextarea.value ? '\n' : '') + md + '\n';
        }
    } catch (_) {
        // fallback toast
        notify(url, 'info');
    }
}

// Modal-based confirmation dialog (Promise<boolean>) using Bootstrap 5
async function confirmModal(message = '确定要执行该操作吗？', options = {}) {
    const opts = {
        title: options.title || '确认操作',
        confirmText: options.confirmText || '确定',
        cancelText: options.cancelText || '取消',
        confirmVariant: options.confirmVariant || 'primary', // 'danger' | 'warning' | 'primary'
        backdrop: 'static',
        keyboard: true
    };
    // Fallback when Bootstrap is unavailable
    if (!window.bootstrap || !window.bootstrap.Modal) {
        return Promise.resolve(window.confirm(message));
    }
    return new Promise((resolve) => {
        const id = 'confirm-modal-' + Date.now() + '-' + Math.random().toString(16).slice(2);
        const wrap = document.createElement('div');
        wrap.className = 'modal fade';
        wrap.id = id;
        wrap.tabIndex = -1;
        wrap.innerHTML = `
            <div class="modal-dialog modal-dialog-centered">
                <div class="modal-content">
                    <div class="modal-header">
                        <h5 class="modal-title">${opts.title}</h5>
                        <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                    </div>
                    <div class="modal-body">
                        <p class="mb-0">${message}</p>
                    </div>
                    <div class="modal-footer">
                        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">${opts.cancelText}</button>
                        <button type="button" class="btn btn-${opts.confirmVariant}" data-role="confirm-btn">${opts.confirmText}</button>
                    </div>
                </div>
            </div>`;
        document.body.appendChild(wrap);
        const modal = window.bootstrap.Modal.getOrCreateInstance(wrap, { backdrop: opts.backdrop, keyboard: opts.keyboard });
        let settled = false;
        const cleanup = () => {
            try { wrap.parentNode && wrap.parentNode.removeChild(wrap); } catch(_) {}
        };
        wrap.addEventListener('hidden.bs.modal', () => {
            if (!settled) { settled = true; resolve(false); }
            // Delay cleanup a tick to avoid interfering with Bootstrap internals
            setTimeout(cleanup, 0);
        });
        const confirmBtn = wrap.querySelector('[data-role="confirm-btn"]');
        if (confirmBtn) {
            confirmBtn.addEventListener('click', () => {
                if (settled) return;
                settled = true;
                resolve(true);
                try { modal.hide(); } catch(_) {}
            });
        }
        try { modal.show(); } catch (e) { settled = true; resolve(window.confirm(message)); cleanup(); }
    });
}

// Pretty URL helpers for categories and posts
function slugForCategory(cat) {
    switch (cat) {
        case '综合': return 'complex';
        case '技术': return 'tech';
        case '评测': return 'review';
        case '线报': return 'report';
        case '推广': return 'promotion';
        case '交易': return 'trade';
        default: return '';
    }
}

function categoryFromSlug(slug) {
    const map = { complex: '综合', tech: '技术', review: '评测', report: '线报', promotion: '推广', trade: '交易' };
    return map[slug] || '';
}

function navigateHome(replace=false) {
    const url = '/';
    if (replace) history.replaceState({ view: 'home' }, '', url);
    else history.pushState({ view: 'home' }, '', url);
    filterByCategory('');
}

function navigateCategory(cat, replace=false) {
    currentCategory = cat || '';
    const slug = slugForCategory(currentCategory);
    const url = slug ? `/categories/${slug}` : '/';
    if (replace) history.replaceState({ view: 'category', category: currentCategory }, '', url);
    else history.pushState({ view: 'category', category: currentCategory }, '', url);
    filterByCategory(currentCategory);
}

function navigatePost(postId, page = 1, replace=false) {
    const p = Math.max(1, Number(page) || 1);
    const url = `/post-${postId}-${p}`;
    if (replace) history.replaceState({ view: 'post', id: postId, page: p }, '', url);
    else history.pushState({ view: 'post', id: postId, page: p }, '', url);
    showPostDetail(postId, p);
}

// Anchor click helper: keep SPA on normal left-click; allow cmd/ctrl/middle clicks to open new tab/window
function handlePostLinkClick(event, postId, page = 1) {
    try {
        // e.button === 1: middle click; meta/ctrl/shift/alt usually intend new tab/window
        if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button === 1) {
            return true; // allow browser default
        }
        event.preventDefault();
        navigatePost(postId, page);
        return false;
    } catch (_) {
        // fallback to default
        return true;
    }
}

// Category link helper: allow right-click/open in new tab; use SPA on normal click
function handleCategoryLinkClick(event, category) {
    try {
        if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button === 1) {
            return true;
        }
        event.preventDefault();
        navigateCategory(category);
        return false;
    } catch (_) {
        return true;
    }
}

// Note: user links now navigate directly to /personal/{username}

function routeFromLocation() {
    try {
        const path = (location.pathname || '/');
        // Support deep-link search via /?search=keyword
        if (path === '/' || path === '/index.html') {
            try {
                const params = new URLSearchParams(location.search || '');
                const q = (params.get('search') || '').trim();
                searchQuery = q;
                if (q) setPageTitle(`搜索: ${q}`);
            } catch (_) { /* ignore */ }
            navigateHome(true);
            return;
        }
    let m = path.match(/^\/categories\/([a-z0-9-]+)\/?$/);
        if (m) {
            const cat = categoryFromSlug(m[1]);
            navigateCategory(cat, true);
            return;
        }
        m = path.match(/^\/post-(\d+)-(\d+)$/);
        if (m) {
            const id = parseInt(m[1], 10);
            const p = parseInt(m[2], 10) || 1;
            // Clear search context when going to a post
            searchQuery = '';
            navigatePost(id, p, true);
            return;
        }
        // legacy user route removed
        // fallback
        // Clear search on unknown routes
        searchQuery = '';
        navigateHome(true);
    } catch (_) {
        navigateHome(true);
    }
}

function setPageTitle(subtitle = '') {
    if (subtitle && subtitle.trim()) {
        document.title = `${subtitle} - ${BASE_TITLE}`;
    } else {
        document.title = `${BASE_TITLE} - ${HOME_TAGLINE}`;
    }
}

// Render Markdown safely via marked (supports both marked() and marked.parse())
function renderMarkdown(md) {
    try {
        const m = window.marked;
        if (!m) return md || '';
        // Try function-style (old), then parse (new), then namespace.marked
        const parser =
            (typeof m === 'function' && m)
            || (typeof m.parse === 'function' && m.parse.bind(m))
            || (m.marked && typeof m.marked === 'function' && m.marked.bind(m));
        return parser ? parser(md || '') : (md || '');
    } catch (e) {
        console.error('markdown render failed:', e);
        return md || '';
    }
}

function displayName(user) {
    return user.username || '未知';
}

function getToken() {
    return localStorage.getItem('token');
}

function setToken(token) {
    localStorage.setItem('token', token);
}
// UI error helpers for register form
function showRegisterError(msg) {
    try {
        const box = document.getElementById('register-alert');
        if (!box) return;
        if (!msg) {
            box.classList.add('d-none');
            box.textContent = '';
            return;
        }
        box.textContent = msg;
        box.classList.remove('d-none');
    } catch (_) {}
}

function markInvalid(el, message) {
    if (!el) return;
    try {
        el.classList.add('is-invalid');
        if (message) el.setAttribute('data-invalid-msg', message);
    } catch (_) {}
}

function clearInvalid(el) {
    if (!el) return;
    try {
        el.classList.remove('is-invalid');
        el.removeAttribute('data-invalid-msg');
    } catch (_) {}
}


function clearToken() {
    localStorage.removeItem('token');
}

async function apiRequest(url, options = {}) {
    const token = getToken();
    if (token) {
        options.headers = { ...options.headers, 'Authorization': `Bearer ${token}` };
    }
    const response = await fetch(url, options);
    if (!response.ok) {
        let msg = `HTTP ${response.status}`;
        try {
            const body = await response.json();
            if (body && (body.message || body.error)) {
                msg = body.message || body.error;
            }
        } catch (_) {}
        throw new Error(msg);
    }
    return await response.json();
}

async function fetchPosts(page = 1, pageSize = 10, search = '') {
    try {
        let url = `${API_BASE}/posts?page=${page}&page_size=${pageSize}`;
        if (search) {
            url += `&search=${encodeURIComponent(search)}`;
        }
        if (currentCategory) {
            url += `&category=${encodeURIComponent(currentCategory)}`;
        }
        const data = await apiRequest(url);
        return data.data || { items: [], pagination: {} };
    } catch (error) {
        console.error('Error fetching posts:', error);
        return { items: [], pagination: {} };
    }
}

function filterByCategory(cat) {
    currentCategory = cat || '';
    currentPage = 1;
    // Title will be finalized in showHome based on currentCategory/searchQuery
    setPageTitle(cat ? cat : BASE_TITLE);
    showHome();
    // 高亮当前分类
    try {
        const isAll = currentCategory === '';
        document.querySelectorAll('[data-home]')
            .forEach(a => a.classList.toggle('active', isAll));
        document.querySelectorAll('[data-category]')
            .forEach(a => a.classList.toggle('active', !isAll && (a.getAttribute('data-category')||'') === currentCategory));
    } catch (_) {}
}

async function fetchPost(id) {
    try {
        const data = await apiRequest(`${API_BASE}/posts/${id}`);
        // 兼容不同结构 {data:{post}}, {post}, 或直接对象
        return data?.data?.post || data?.post || data?.data || data;
    } catch (error) {
        console.error('Error fetching post:', error);
        return null;
    }
}

async function updateStatsFromAPI(fallbackTotal = 0) {
    try {
        // 后端路由是 GET /api/v1/stats
        const data = await apiRequest(`${API_BASE}/stats`);
        const stats = data?.data || data || {};
        document.getElementById('post-count').textContent = stats.post_count ?? stats.posts ?? fallbackTotal ?? 0;
        document.getElementById('user-count').textContent = stats.user_count ?? stats.users ?? 0;
        document.getElementById('daily-active-count').textContent = stats.daily_active ?? stats.daily_active_count ?? stats.daily_active_users ?? 0;
    } catch (error) {
        console.error('Error loading stats:', error);
        document.getElementById('post-count').textContent = fallbackTotal ?? 0;
    }
}

function renderPosts(posts, pagination = {}) {
    const postsDiv = document.getElementById('posts');
    if (!postsDiv) {
        // 容器已被其他视图（例如帖子详情）替换，放弃本次渲染以避免报错
        return;
    }
    postsDiv.innerHTML = '';
    if (!posts || posts.length === 0) {
        postsDiv.innerHTML = '<p class="text-muted">暂无帖子</p>';
        return;
    }
    posts.forEach(post => {
        const col = document.createElement('div');
        col.className = 'col-md-12 mb-3';
        const authorName = displayName(post.author || post.user);
        const authorUsername = (post.author && post.author.username) || (post.user && post.user.username) || '';
        const authorHref = authorUsername ? `http://127.0.0.1:8080/personal/${encodeURIComponent(authorUsername)}` : '#';
        const createdAt = safeDate(post.created_at);
    const cat = post.category || '综合';
    const catSlug = slugForCategory(cat);
    const metaLine = `👤 <a href="${authorHref}" style="text-decoration: none; color: inherit;">${authorName}</a>${createdAt ? ` · 🕒 ${createdAt}` : ''} · 📂 <a href="${catSlug ? '/categories/' + catSlug : '/'}" onclick="return handleCategoryLinkClick(event, '${cat}')" style="text-decoration: none; color: inherit;">${cat}</a>`;
        col.innerHTML = `
            <div class="card post-card">
                <div class="card-body">
                    <h5 class="card-title" style="cursor: pointer;">
                        <a href="/post-${post.id}-1" onclick="return handlePostLinkClick(event, ${post.id}, 1)" style="text-decoration: none; color: inherit;">${post.title}</a>
                    </h5>
                    <p class="card-text">${(post.content || '').substring(0, 200)}${post.content && post.content.length > 200 ? '...' : ''}</p>
                    <p class="card-text"><small class="text-muted">${metaLine}<span id="post-stats-${post.id}"></span></small></p>
                </div>
            </div>
        `;
        postsDiv.appendChild(col);
        // 异步加载该帖的回帖数与PV
        loadPostStats(post.id);
    });

    // Add pagination
    if (pagination.total_pages > 1) {
        const paginationDiv = document.createElement('div');
        paginationDiv.className = 'd-flex justify-content-end w-100 mt-4';
        paginationDiv.innerHTML = `
            <nav>
                <ul class="pagination">
                    ${pagination.page > 1 ? `<li class="page-item"><a class="page-link" href="#" onclick="changePage(${pagination.page - 1}); return false;">上一页</a></li>` : ''}
                    ${Array.from({length: pagination.total_pages}, (_, i) => i + 1).map(p => 
                        `<li class="page-item ${p === pagination.page ? 'active' : ''}"><a class="page-link" href="#" onclick="changePage(${p}); return false;">${p}</a></li>`
                    ).join('')}
                    ${pagination.page < pagination.total_pages ? `<li class="page-item"><a class="page-link" href="#" onclick="changePage(${pagination.page + 1}); return false;">下一页</a></li>` : ''}
                </ul>
            </nav>
        `;
        postsDiv.appendChild(paginationDiv);
    }
}

function safeDate(ts) {
    // 支持字符串或时间戳，失败则返回""
    const d = ts ? new Date(ts) : null;
    return d && !isNaN(d.getTime()) ? d.toLocaleString() : '';
}

function renderPostDetail(post) {
    if (!post) {
    notify('未找到帖子', 'error', 4000);
        showHome();
        return;
    }
    setPageTitle(post.title || '帖子详情');
    const author = post.author || post.user || {};
    const authorName = displayName(author);
    const authorUsername = author.username || '';
    const authorHref = authorUsername ? `http://127.0.0.1:8080/personal/${encodeURIComponent(authorUsername)}` : '#';
    const createdLabel = safeDate(post.created_at) ? ` | 🕒 ${safeDate(post.created_at)}` : '';
    const cat = post.category || '综合';
    const catSlug = slugForCategory(cat);
    const isAuthor = currentUser && currentUser.id === post.user_id;
    const isAdmin = !!(currentUser && currentUser.is_admin);
    const contentDiv = document.getElementById('content');
    // 附件已直接插入正文，不再在文末重复展示
    const totalComments = Array.isArray(post.comments) ? post.comments.length : 0;
    const totalCommentPages = Math.max(1, Math.ceil(totalComments / COMMENTS_PAGE_SIZE));
    const currentCPage = Math.min(currentCommentsPage, totalCommentPages);
    const startIdx = (currentCPage - 1) * COMMENTS_PAGE_SIZE;
    const endIdx = startIdx + COMMENTS_PAGE_SIZE;

    const pager = totalCommentPages > 1
        ? `<nav><ul class="pagination mb-0">${Array.from({length: totalCommentPages}, (_, i) => i + 1)
            .map(p => `<li class="page-item ${p===currentCPage?'active':''}"><a class="page-link" href="#" onclick="changeCommentsPage(${post.id}, ${p}); return false;">${p}</a></li>`)
            .join('')}</ul></nav>`
        : '';

    contentDiv.innerHTML = `
        <div class="card">
            <div class="card-body">
                <h2 class="card-title">${post.title}</h2>
                <div class="card-text">${DOMPurify.sanitize(renderMarkdown(post.content || ''))}</div>
                
                <p class="card-text"><small class="text-muted">👤 <a href="${authorHref}" style="text-decoration: none; color: inherit;">${authorName}</a>${createdLabel} · 📂 <a href="${catSlug ? '/categories/' + catSlug : '/'}" onclick="return handleCategoryLinkClick(event, '${cat}')" style="text-decoration: none; color: inherit;">${cat}</a></small></p>
                ${(isAuthor || isAdmin) ? `<div class="mt-3">${isAuthor ? `<button class=\"btn btn-warning me-2\" onclick=\"editPost(${post.id})\">编辑</button>` : ''}<button class=\"btn btn-danger\" onclick=\"deletePost(${post.id})\">删除</button></div>` : ''}
            </div>
        </div>
        <h4 class="mt-4">评论</h4>
        <div id="comments"></div>
        ${totalCommentPages > 1 ? `<div class="d-flex justify-content-end mt-3">${pager}</div>` : ''}
        ${currentUser ? '<div class="mt-3"><textarea class="form-control" id="comment-content" placeholder="写评论..."></textarea><button id="comment-submit-' + post.id + '" class="btn btn-primary mt-2" onclick="submitComment(' + post.id + ')">提交评论</button></div>' : '<p class="mt-3">登录后可评论</p>'}
    `;
    if (post.comments) {
        const commentsDiv = document.getElementById('comments');
        post.comments.slice(startIdx, endIdx).forEach(comment => {
            const authorObj = comment.author || comment.user || {};
            const commentAuthor = displayName(authorObj);
            const authorUsername = authorObj.username || '';
            const authorHref = authorUsername ? `http://127.0.0.1:8080/personal/${encodeURIComponent(authorUsername)}` : '#';
            const commentLabel = safeDate(comment.created_at) ? ` | 🕒 ${safeDate(comment.created_at)}` : '';
            const commentDiv = document.createElement('div');
            commentDiv.className = 'card mt-2';
            commentDiv.id = `comment-card-${comment.id}`;
            const canDelete = !!(currentUser && (currentUser.is_admin || currentUser.id === comment.user_id));
            commentDiv.innerHTML = `
                <div class="card-body">
                    <p class="card-text">${DOMPurify.sanitize(comment.content || '')}</p>
                    <p class="card-text d-flex justify-content-between align-items-center">
                        <small class="text-muted">👤 <a href="${authorHref}" style="text-decoration: none; color: inherit;">${commentAuthor}</a>${commentLabel}</small>
                        ${canDelete ? `<button class="btn btn-sm btn-outline-danger" onclick="deleteComment(${comment.id})">删除</button>` : ''}
                    </p>
                </div>
            `;
            commentsDiv.appendChild(commentDiv);
        });
    }
}

// Legacy user page functions removed in favor of personal page

function changeCommentsPage(postId, page) {
    currentCommentsPage = Math.max(1, Number(page) || 1);
    navigatePost(postId, currentCommentsPage);
}

async function submitComment(postId) {
    if (!currentUser) {
    notify('请先登录', 'warning');
        showLogin();
        return;
    }
    const textarea = document.getElementById('comment-content');
    if (!textarea) { return; }
    const content = (textarea.value || '').trim();
    if (!content) {
    notify('请输入评论内容', 'warning');
        return;
    }
    const btn = document.getElementById(`comment-submit-${postId}`);
    const origText = btn ? btn.textContent : '';
    try {
        if (btn) { btn.disabled = true; btn.textContent = '提交中...'; }
        const resp = await fetch(`${API_BASE}/posts/${postId}/comments`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${getToken()}` },
            body: JSON.stringify({ content })
        });
        const data = await resp.json();
        if (!resp.ok || (data && data.code && data.code !== 0)) {
            const msg = data?.message || `HTTP ${resp.status}`;
            notify('评论失败：' + msg, 'error', 4000);
            return;
        }
        // 局部插入新评论，不整帖刷新
        const comment = data?.data?.comment || data?.comment || data;
        const commentsDiv = document.getElementById('comments');
        if (commentsDiv && comment) {
            const authorObj = comment.author || comment.user || currentUser || {};
            const commentAuthor = displayName(authorObj);
            const authorUsername = authorObj.username || '';
            const authorHref = authorUsername ? `http://127.0.0.1:8080/personal/${encodeURIComponent(authorUsername)}` : '#';
            const commentLabel = safeDate(comment.created_at) ? ` | 🕒 ${safeDate(comment.created_at)}` : '';
            const commentDiv = document.createElement('div');
            commentDiv.className = 'card mt-2';
            commentDiv.id = `comment-card-${comment.id}`;
            commentDiv.innerHTML = `
                <div class="card-body">
                    <p class="card-text">${DOMPurify.sanitize(comment.content || '')}</p>
                    <p class="card-text d-flex justify-content-between align-items-center">
                        <small class="text-muted">👤 <a href="${authorHref}" style="text-decoration: none; color: inherit;">${commentAuthor}</a>${commentLabel}</small>
                        ${currentUser ? `<button class="btn btn-sm btn-outline-danger" onclick="deleteComment(${comment.id})">删除</button>` : ''}
                    </p>
                </div>
            `;
            // 插到顶部
            if (commentsDiv.firstChild) {
                commentsDiv.insertBefore(commentDiv, commentsDiv.firstChild);
            } else {
                commentsDiv.appendChild(commentDiv);
            }
            // 如果当前在第1页，保持最多 COMMENTS_PAGE_SIZE 条，超出则移除最后一个
            try {
                if (currentCommentsPage === 1) {
                    const items = commentsDiv.querySelectorAll('.card.mt-2');
                    if (items.length > COMMENTS_PAGE_SIZE) {
                        commentsDiv.removeChild(items[items.length - 1]);
                    }
                }
            } catch (_) {}
        }
        textarea.value = '';
    } catch (e) {
    notify('评论失败：' + e.message, 'error', 4000);
    } finally {
        if (btn) { btn.disabled = false; btn.textContent = origText || '提交评论'; }
    }
}

async function submitPost(title, content, category, attachments) {
    try {
        await apiRequest(`${API_BASE}/posts`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ title, content, category, attachments })
        });
        showHome(); // Refresh
    } catch (error) {
    notify('发帖失败: ' + error.message, 'error', 4000);
    }
}

async function deleteComment(commentId) {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    const ok = await confirmModal('确定要删除此评论吗？', { title: '删除确认', confirmText: '删除', confirmVariant: 'danger' });
    if (!ok) return;
    try {
        const resp = await fetch(`${API_BASE}/comments/${commentId}`, { method: 'DELETE', headers: { 'Authorization': `Bearer ${getToken()}` } });
        const data = await resp.json();
        if (!resp.ok || (data && data.code && data.code !== 0)) {
            const msg = data?.message || `HTTP ${resp.status}`;
            notify('删除失败：' + msg, 'error', 4000);
            return;
        }
        const el = document.getElementById(`comment-card-${commentId}`);
        if (el && el.parentNode) el.parentNode.removeChild(el);
    } catch (e) {
        notify('删除失败：' + e.message, 'error', 4000);
    }
}

async function login(username, password) {
    try {
        const data = await fetch(`${API_BASE}/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ username, password })
        }).then(r => r.json());
        if (data.data && data.data.token) {
            setToken(data.data.token);
            currentUser = data.data.user; // 包含 is_admin
            updateUI();
        } else {
            notify('登录失败: ' + (data.message || '未知错误'), 'error', 4000);
        }
    } catch (error) {
    notify('登录失败: ' + error.message, 'error', 4000);
    }
}

async function register(username, email, password, confirm, code) {
    try {
        const payload = { username, email, password, confirm, code };
        // attach captcha if visible
        try {
            const wrap = document.getElementById('captcha-wrap');
            const idEl = document.getElementById('captcha-id');
            const ansEl = document.getElementById('captcha-answer');
            if (wrap && wrap.style.display !== 'none' && idEl && ansEl) {
                const cid = (idEl.value || '').trim();
                const cans = (ansEl.value || '').trim();
                if (cid && cans) {
                    payload.captcha_id = cid;
                    payload.captcha_answer = cans;
                }
            }
        } catch (_) {}
        const data = await fetch(`${API_BASE}/auth/register`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload)
        }).then(r => r.json());
        if (data.data && data.data.token) {
            setToken(data.data.token);
            currentUser = data.data.user; // 包含 is_admin
            updateUI();
            notify('注册成功，已自动登录', 'success');
        } else {
            notify('注册失败: ' + (data.message || '未知错误'), 'error', 4000);
        }
    } catch (error) {
    notify('注册失败: ' + error.message, 'error', 4000);
    }
}

async function loadCaptchaIfEnabled() {
    try {
        // 简单探测：直接请求 captcha 接口，若成功则显示。失败则隐藏。
        const res = await fetch(`${API_BASE}/auth/captcha`);
        const data = await res.json();
        const wrap = document.getElementById('captcha-wrap');
        const img = document.getElementById('captcha-image');
        const idEl = document.getElementById('captcha-id');
        if (!res.ok || !data || !(data.data || data).id) {
            if (wrap) wrap.style.display = 'none';
            return;
        }
        const payload = data.data || data;
        if (wrap) wrap.style.display = 'block';
        if (img) { img.src = payload.image; img.style.display = 'inline-block'; }
        if (idEl) idEl.value = payload.id;
    } catch (_) {
        const wrap = document.getElementById('captcha-wrap');
        if (wrap) wrap.style.display = 'none';
    }
}

async function refreshCaptcha() {
    try {
        const btn = document.getElementById('btn-refresh-captcha');
        if (btn) { btn.disabled = true; btn.textContent = '刷新中...'; }
        const res = await fetch(`${API_BASE}/auth/captcha`);
        const data = await res.json();
        const payload = data.data || data;
        const img = document.getElementById('captcha-image');
        const idEl = document.getElementById('captcha-id');
        if (payload && payload.image && payload.id) {
            if (img) { img.src = payload.image; img.style.display = 'inline-block'; }
            if (idEl) idEl.value = payload.id;
        }
        if (btn) { btn.disabled = false; btn.textContent = '刷新'; }
    } catch (_) {
        const btn = document.getElementById('btn-refresh-captcha');
        if (btn) { btn.disabled = false; btn.textContent = '刷新'; }
    }
}

function logout() {
    clearToken();
    currentUser = null;
    updateUI();
}

async function dailySignIn() {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    const btn = document.getElementById('btn-signin');
    const orig = btn ? btn.textContent : '';
    try {
        if (btn) { btn.disabled = true; btn.textContent = '签到中...'; }
        const res = await fetch(`${API_BASE}/signin/daily`, { method: 'POST', headers: { 'Authorization': `Bearer ${getToken()}` } });
        const data = await res.json();
        if (res.ok && (data.code === 0 || data.message === 'sign-in successful')) {
            notify('签到成功' + (data.data?.points_awarded ? `，积分+${data.data.points_awarded}` : ''), 'success');
            try { await refreshSigninStatus(); } catch(_) {}
        } else {
            const msg = data?.message || '签到失败';
            notify(msg, 'error', 4000);
        }
    } catch (e) {
        notify('签到失败：' + (e.message || '网络异常'), 'error', 4000);
    } finally {
        if (btn) { btn.disabled = false; btn.textContent = orig || '签到'; }
    }
}

function updateUI(doRender = true) {
    const userInfo = document.getElementById('user-info');
    const loginForm = document.querySelector('.login-form');
    const registerForm = document.querySelector('.register-form');
    if (currentUser) {
        userInfo.style.display = 'block';
        loginForm.style.display = 'none';
        registerForm.style.display = 'none';
        const name = displayName(currentUser);
        const isAdmin = !!currentUser.is_admin;
        const badge = isAdmin ? ' <span class="badge bg-danger ms-1">管理员</span>' : '';
        const el = document.getElementById('user-details');
        if (el) el.innerHTML = `您好，${name}${badge}`;
        const createBtn = document.getElementById('create-post-btn');
        if (createBtn) createBtn.disabled = false;
        try { refreshSigninStatus(); } catch(_) {}
    } else {
        userInfo.style.display = 'none';
        loginForm.style.display = 'block';
        registerForm.style.display = 'none';
    }
    // 避免页面初次加载时与路由渲染产生竞争：只在需要时渲染首页
    if (doRender) {
        showHome();
    }
}

async function refreshSigninStatus() {
    try {
        const res = await fetch(`${API_BASE}/signin/status`, { headers: { 'Authorization': `Bearer ${getToken()}` } });
        const data = await res.json();
        if (!res.ok) return;
        const payload = data.data || data;
        const pts = typeof payload.points === 'number' ? payload.points : null;
        if (pts !== null) {
            const el = document.getElementById('user-points');
            if (el) el.textContent = pts;
        }
    } catch(_) {}
}

function showLogin() {
    const loginForm = document.querySelector('.login-form');
    const registerForm = document.querySelector('.register-form');
    loginForm.style.display = 'block';
    registerForm.style.display = 'none';
}

function showRegister() {
    // 新注册页改为独立路由
    try { window.location.href = '/register'; } catch(_) {}
}

function goMyPersonal() {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    const username = currentUser.username || '';
    if (username) {
        window.location.href = `http://127.0.0.1:8080/personal/${encodeURIComponent(username)}`;
    } else {
        notify('未找到用户名', 'warning');
    }
}

function changePage(page) {
    currentPage = page;
    showHome();
}

async function showUsers() {
    setPageTitle('用户列表');
    const contentDiv = document.getElementById('content');
    contentDiv.innerHTML = '<h2 class="mb-4">用户列表</h2><div id="users"></div>';
    try {
        const res = await apiRequest(`${API_BASE}/users?page=1&page_size=20`);
        const items = res?.data?.items || res?.items || [];
        const table = document.createElement('table');
        table.className = 'table table-striped';
        table.innerHTML = `
            <thead><tr><th>ID</th><th>用户名</th><th>邮箱</th><th>注册IP</th><th>注册时间</th></tr></thead>
            <tbody>
                ${items.map(u => `<tr><td>${u.id}</td><td>${u.username}</td><td>${u.email||''}</td><td>${u.register_ip||''}</td><td>${safeDate(u.created_at)}</td></tr>`).join('')}
            </tbody>`;
        document.getElementById('users').appendChild(table);
    } catch (e) {
        notify('加载用户失败: ' + e.message, 'error', 4000);
    }
}

async function showHome() {
    // Set title by context: search > category > default
    if (searchQuery) {
        setPageTitle(`搜索: ${searchQuery}`);
    } else if (currentCategory) {
        setPageTitle(currentCategory);
    } else {
        setPageTitle();
    }
    currentListContext = { type: currentCategory ? 'category' : 'home' };
    const contentDiv = document.getElementById('content');
    contentDiv.innerHTML = '<div id="posts" class="row"></div>';

    // 防御：根据当前 URL 修正 currentCategory，避免偶发状态丢失导致刷新时回到“全部”
    try {
        const path = (location.pathname || '/');
        const m = path.match(/^\/categories\/([a-z0-9-]+)\/?$/);
        if (m) {
            const cat = categoryFromSlug(m[1]);
            if (cat) currentCategory = cat;
        }
    } catch (_) {}

    const postsData = await fetchPosts(currentPage, 10, searchQuery);
    renderPosts(postsData.items, postsData.pagination);
    updateStatsFromAPI(postsData.pagination.total || 0);
    // 初始进入首页时，高亮“首页”（全部）
    try {
        const isAll = !currentCategory;
        document.querySelectorAll('[data-home]')
            .forEach(a => a.classList.toggle('active', isAll));
        document.querySelectorAll('[data-category]')
            .forEach(a => a.classList.toggle('active', !isAll && (a.getAttribute('data-category')||'') === currentCategory));
    } catch (_) {}
}

function showPostDetail(postId, commentPage = 1) {
    currentCommentsPage = Math.max(1, Number(commentPage) || 1);
    setPageTitle('帖子详情');
    const content = document.getElementById('content');
    content.innerHTML = '<h2>加载中...</h2>';
    fetchPost(postId).then(post => {
        if (!post) {
            content.innerHTML = '<div class="alert alert-danger">加载失败或帖子不存在</div>';
            const backBtn = document.createElement('button');
            backBtn.className = 'btn btn-secondary mt-2';
            backBtn.textContent = '返回列表';
            backBtn.onclick = () => navigateCategory('');
            content.appendChild(backBtn);
            return;
        }
        renderPostDetail(post);
    }).catch(err => {
        console.error('load post failed:', err);
        content.innerHTML = '<div class="alert alert-danger">加载失败，请返回列表重试</div>';
        const backBtn = document.createElement('button');
        backBtn.className = 'btn btn-secondary mt-2';
        backBtn.textContent = '返回列表';
    backBtn.onclick = () => navigateCategory('');
        content.appendChild(backBtn);
    });
}

function showCreatePostPage() {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    setPageTitle('发布新帖子');
    const contentDiv = document.getElementById('content');
    contentDiv.innerHTML = `
        <button class="btn btn-secondary mb-3" onclick="showHome()">返回列表</button>
        <div class="card">
            <div class="card-body">
                <h2 class="card-title">发布新帖子</h2>
                <form id="create-post-form">
                    <div class="mb-3">
                        <input type="text" class="form-control" id="create-post-title" placeholder="帖子标题" required>
                    </div>
                    <div class="mb-3">
                        <select class="form-control" id="create-post-category" required>
                            <option value="综合">综合</option>
                            <option value="评测">评测</option>
                            <option value="技术">技术</option>
                            <option value="线报">线报</option>
                            <option value="推广">推广</option>
                            <option value="交易">交易</option>
                        </select>
                    </div>
                    <div class="mb-3">
                        <textarea class="form-control" id="create-post-content" rows="5" placeholder="帖子内容"></textarea>
                    </div>
                    <div class="mb-3">
                        <input type="file" class="form-control" id="create-post-attachment" accept="image/*" multiple>
                        <div class="form-text text-muted" id="attachment-hint">上传的附件将以外链形式插入正文</div>
                        <div id="attachment-preview" class="mt-2"></div>
                    </div>
                    <button type="submit" class="btn btn-success">提交</button>
                </form>
            </div>
        </div>
    `;
    // 根据当前分类自动选中（例如 /categories/review 则 currentCategory 为“评测”）
    try {
        const sel = document.getElementById('create-post-category');
        if (sel && currentCategory) {
            sel.value = currentCategory;
        }
    } catch (_) {}
    const form = document.getElementById('create-post-form');
    const textarea = document.getElementById('create-post-content');
    
    // Initialize EasyMDE for Markdown editing
    let mde;
    try {
        mde = new EasyMDE({
            element: textarea,
            spellChecker: false,
            placeholder: '支持 Markdown 语法，鼓励友善发言，禁止人身攻击',
            autosave: { enabled: false },
            toolbar: [
                'bold', 'italic', 'heading', '|',
                'code', 'quote', 'unordered-list', 'ordered-list', '|',
                'link', 'image', '|',
                'preview', 'side-by-side', 'fullscreen'
            ],
            status: ['autosave', 'lines', 'words', 'cursor'],
            renderingConfig: {
                singleLineBreaks: false,
                codeSyntaxHighlighting: true,
            }
        });
    } catch (error) {
        console.error('Failed to initialize EasyMDE:', error);
        // Fallback to simple textarea
        mde = { value: () => textarea.value };
        notify('Markdown 编辑器加载失败，使用普通文本编辑', 'warning', 5000);
    }
    // Track uploaded attachment URLs for backend record (content已直接插入URL)
    const pendingAttachments = [];
    // Auto-upload on file selection and insert into editor content
    try {
        const attachmentInput = document.getElementById('create-post-attachment');
        const hint = document.getElementById('attachment-hint');
        const preview = document.getElementById('attachment-preview');
        if (attachmentInput) {
            attachmentInput.addEventListener('change', async () => {
                const files = attachmentInput.files || [];
                if (!files.length) {
                    if (hint) hint.textContent = '上传的附件将以外链形式插入正文';
                    return;
                }
                if (hint) hint.textContent = '上传中...';
                preview && (preview.innerHTML = '');
                for (const file of files) {
                    try {
                        const url = await uploadFileToLocal(file);
                        pendingAttachments.push(url);
                        insertUrlIntoEditor(mde, url, file.name || 'image');
                        // small preview
                        if (preview) {
                            const img = document.createElement('img');
                            img.src = url; img.className = 'img-fluid me-2 mb-2'; img.style.maxWidth = '180px';
                            preview.appendChild(img);
                        }
                        notify(`已上传: ${file.name}`, 'success');
                    } catch (e) {
                        notify(`上传失败: ${file.name} - ${e.message}`, 'error', 5000);
                    }
                }
                if (hint) hint.textContent = files.length ? `已插入 ${files.length} 个附件` : '上传的附件将以外链形式插入正文';
                // clear selection
                attachmentInput.value = '';
            });
        }
    } catch (_) {}
    
    if (form) {
        form.addEventListener('submit', async (event) => {
            event.preventDefault();
            const title = document.getElementById('create-post-title').value.trim();
            const category = document.getElementById('create-post-category').value;
            const content = mde.value().trim();

            if (!title || !content) {
                notify('请填写标题和内容', 'warning');
                return;
            }
            
            if (!getToken()) {
                notify('您尚未登录，请先登录', 'warning');
                showLogin();
                return;
            }

            try {
                await submitPost(title, content, category, JSON.stringify(pendingAttachments));
            } finally {
                form.reset();
                document.getElementById('attachment-preview').innerHTML = '';
                if(mde && typeof mde.value === 'function') {
                    mde.value('');
                } else {
                    textarea.value = '';
                }
            }
        });
    }
}

async function showMyPosts() {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    setPageTitle('我的帖子');
    const contentDiv = document.getElementById('content');
    contentDiv.innerHTML = '<h2 class="mb-4">我的帖子</h2><div id="posts" class="row"></div>';
    try {
        const response = await apiRequest(`${API_BASE}/users/me/posts?page=${currentPage}&page_size=10`);
        const payload = response?.data || { items: [], pagination: {} };
        renderPosts(payload.items || [], payload.pagination || {});
    } catch (error) {
        notify('加载失败: ' + error.message, 'error', 4000);
    }
}

async function showProfile() {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    try {
        const me = await apiRequest(`${API_BASE}/auth/me`);
        currentUser = me?.data || me || currentUser;
    } catch (_) {}
    const username = (currentUser && currentUser.username) ? currentUser.username : '';
    if (username) {
        // 跳转到全新的个人页面
        window.location.href = `http://127.0.0.1:8080/personal/${encodeURIComponent(username)}`;
    } else {
        notify('未找到用户名', 'warning');
    }
}

async function editPost(postId) {
    if (!currentUser) {
        notify('请先登录', 'warning');
        showLogin();
        return;
    }
    setPageTitle('编辑帖子');
    const contentDiv = document.getElementById('content');
    contentDiv.innerHTML = `
        <button class="btn btn-secondary mb-3" onclick="showPostDetail(${postId})">返回帖子</button>
        <div class="card">
            <div class="card-body">
                <h2 class="card-title">编辑帖子</h2>
                <form id="edit-post-form">
                    <div class="mb-3">
                        <input type="text" class="form-control" id="edit-post-title" required>
                    </div>
                    <div class="mb-3">
                        <select class="form-control" id="edit-post-category" required>
                            <option value="综合">综合</option>
                            <option value="评测">评测</option>
                            <option value="技术">技术</option>
                            <option value="线报">线报</option>
                            <option value="推广">推广</option>
                            <option value="交易">交易</option>
                        </select>
                    </div>
                    <div class="mb-3">
                        <textarea class="form-control" id="edit-post-content" rows="5" required></textarea>
                    </div>
                    <div class="mb-3">
                        <input type="file" class="form-control" id="edit-post-attachment" accept="image/*" multiple>
                        <div class="form-text text-muted" id="edit-attachment-hint">上传的附件将以外链形式插入正文</div>
                        <div id="edit-attachment-preview" class="mt-2"></div>
                    </div>
                    <button type="submit" class="btn btn-warning">保存更改</button>
                </form>
            </div>
        </div>
    `;

    // 获取最新帖子数据以填充表单，避免因内联模板注入导致的引号/换行破坏
    let post;
    try {
        const data = await apiRequest(`${API_BASE}/posts/${postId}`);
        post = data?.data?.post || data?.post || data?.data || data;
    } catch (e) {
        console.error('加载帖子失败:', e);
        notify('加载帖子失败', 'error', 4000);
        showPostDetail(postId);
        return;
    }

    // 安全填充
    const titleInput = document.getElementById('edit-post-title');
    const categorySelect = document.getElementById('edit-post-category');
    const contentTextarea = document.getElementById('edit-post-content');
    if (titleInput) titleInput.value = post.title || '';
    if (categorySelect) categorySelect.value = post.category || '综合';
    if (contentTextarea) contentTextarea.value = post.content || '';

    const form = document.getElementById('edit-post-form');
    const editMde = new EasyMDE({
        element: document.getElementById('edit-post-content'),
        spellChecker: false,
        autosave: { enabled: false },
        uploadImage: true,
        imageAccept: 'image/*',
        imageUploadFunction: async (file, onSuccess, onError) => {
            try {
                const formData = new FormData();
                formData.append('file', file);
                const resp = await fetch(`${API_BASE}/upload`, { method: 'POST', headers: { 'Authorization': `Bearer ${getToken()}` }, body: formData });
                const data = await resp.json();
                if (data?.data?.url) onSuccess(data.data.url);
                else onError('upload failed');
            } catch (e) { onError(e.message); }
        }
    });
    // 让 Markdown 编辑器拿到实际内容
    editMde.value(post.content || '');
    // Track new attachments added in edit flow
    const pendingEditAttachments = [];
    // Auto-upload and insert on change
    try {
    const input = document.getElementById('edit-post-attachment');
        const hint = document.getElementById('edit-attachment-hint');
        const preview = document.getElementById('edit-attachment-preview');
        if (input) {
            input.addEventListener('change', async () => {
                const files = input.files || [];
                if (!files.length) { if (hint) hint.textContent = '上传的附件将以外链形式插入正文'; return; }
                if (hint) hint.textContent = '上传中...';
                preview && (preview.innerHTML = '');
                for (const file of files) {
                    try {
                        const url = await uploadFileToLocal(file);
                        pendingEditAttachments.push(url);
                        insertUrlIntoEditor(editMde, url, file.name || 'image');
                        if (preview) {
                            const img = document.createElement('img');
                            img.src = url; img.className = 'img-fluid me-2 mb-2'; img.style.maxWidth = '180px';
                            preview.appendChild(img);
                        }
                        notify(`已上传: ${file.name}`, 'success');
                    } catch (e) {
                        notify(`上传失败: ${file.name} - ${e.message}`, 'error', 5000);
                    }
                }
                if (hint) hint.textContent = files.length ? `已插入 ${files.length} 个附件` : '上传的附件将以外链形式插入正文';
                input.value = '';
            });
        }
    } catch (_) {}
    if (form) {
        form.addEventListener('submit', async (event) => {
            event.preventDefault();
            const newTitle = document.getElementById('edit-post-title').value.trim();
            const newCategory = document.getElementById('edit-post-category').value;
            const newContent = editMde.value().trim();
            if (!newTitle || !newContent) {
                notify('请填写标题和内容', 'warning');
                return;
            }

            try {
                await apiRequest(`${API_BASE}/posts/${postId}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ title: newTitle, content: newContent, category: newCategory, attachments: JSON.stringify(pendingEditAttachments) })
                });
                notify('帖子已更新', 'success');
                showPostDetail(postId);
            } catch (error) {
                notify('更新失败: ' + error.message, 'error', 4000);
            }
        });
    }
}

async function deletePost(postId) {
    const ok = await confirmModal('确定要删除此帖子吗？', { title: '删除确认', confirmText: '删除', confirmVariant: 'danger' });
    if (!ok) return;
    try {
        await apiRequest(`${API_BASE}/posts/${postId}`, { method: 'DELETE' });
        notify('帖子已删除', 'success');
        showHome();
    } catch (error) {
        notify('删除失败: ' + error.message, 'error', 4000);
    }
}

async function loadPostStats(postId) {
    try {
        const data = await apiRequest(`${API_BASE}/posts/${postId}/stats`);
        const stats = data?.data || data;
        const span = document.getElementById(`post-stats-${postId}`);
        if (span && stats) {
            const replies = stats.comments_count ?? stats.reply_count ?? 0;
            const pv = stats.pv ?? stats.view_count ?? 0;
            span.textContent = ` · 💬 ${replies} · 👀 ${pv}`;
        }
    } catch (error) {
        // 静默失败，不阻塞帖子渲染
        const span = document.getElementById(`post-stats-${postId}`);
        if (span) span.textContent = '';
    }
}

async function searchPosts(event) {
    event.preventDefault();
    let query = '';
    try {
        const srcInput = event?.target?.querySelector('input[type="search"]');
        query = (srcInput?.value || '').trim();
        if (!query) {
            query = (document.getElementById('search-input')?.value || document.getElementById('search-input-mobile')?.value || '').trim();
        }
    } catch (_) {
        query = (document.getElementById('search-input')?.value || document.getElementById('search-input-mobile')?.value || '').trim();
    }
    searchQuery = query;
    currentPage = 1;
    const title = query ? `搜索: ${query}` : '搜索';
    setPageTitle(title);
    const contentDiv = document.getElementById('content');
    contentDiv.innerHTML = '<h2 class="mb-4">搜索结果</h2><div id="posts" class="row"></div>';
    const data = await fetchPosts(currentPage, 10, searchQuery);
    renderPosts(data.items, data.pagination);
    updateStatsFromAPI(data.pagination.total || 0);
    try {
        const offcanvasEl = document.getElementById('mobileNav');
        if (offcanvasEl && offcanvasEl.classList.contains('show') && window.bootstrap?.Offcanvas) {
            const api = window.bootstrap.Offcanvas.getOrCreateInstance(offcanvasEl);
            api.hide();
        }
    } catch (_) {}
}

document.addEventListener('DOMContentLoaded', function() {
    const loginFormEl = document.getElementById('login-form');
    if (loginFormEl) {
        loginFormEl.addEventListener('submit', function(event) {
            event.preventDefault();
            const username = document.getElementById('username').value.trim();
            const password = document.getElementById('password').value;
            login(username, password);
        });
    }

    const registerFormEl = document.getElementById('register-form');
    if (registerFormEl) {
        // Blur validations
        const uEl = document.getElementById('reg-username');
        const pEl = document.getElementById('reg-password');
        const cEl = document.getElementById('reg-confirm');
        const capAns = document.getElementById('captcha-answer');
        const capId = document.getElementById('captcha-id');

        if (uEl) uEl.addEventListener('blur', function(){
            const v = (uEl.value||'').trim();
            if (!/^[\p{sc=Han}A-Za-z0-9-]{2,15}$/u.test(v)) {
                markInvalid(uEl, '用户名需为2-15位，仅中文/英文/数字/横杠-');
            } else {
                clearInvalid(uEl);
            }
        });
        if (pEl) pEl.addEventListener('blur', function(){
            const v = pEl.value||'';
            if (!/^[A-Za-z0-9._-]{6,18}$/.test(v)) {
                markInvalid(pEl, '密码需为6-18位，仅包含 a-z A-Z 0-9 和 -_.');
            } else {
                clearInvalid(pEl);
            }
        });
        if (cEl) cEl.addEventListener('blur', function(){
            const v = cEl.value||'';
            if (v !== (pEl?.value||'')) {
                markInvalid(cEl, '两次输入的密码不一致');
            } else {
                clearInvalid(cEl);
            }
        });
        if (capAns) capAns.addEventListener('blur', async function(){
            const id = (capId?.value||'').trim();
            const ans = (capAns.value||'').trim();
            const wrap = document.getElementById('captcha-wrap');
            if (!wrap || wrap.style.display === 'none') return;
            if (!id || !ans) {
                markInvalid(capAns, '请输入验证码');
                return;
            }
            try {
                const res = await fetch(`${API_BASE}/auth/captcha/verify`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ captcha_id: id, captcha_answer: ans }) });
                const data = await res.json();
                if (!res.ok || !(data.code === 0 || data.ok)) {
                    markInvalid(capAns, '验证码不正确');
                } else {
                    clearInvalid(capAns);
                }
            } catch (_) {
                markInvalid(capAns, '验证码校验失败');
            }
        });

        registerFormEl.addEventListener('submit', function(event) {
            event.preventDefault();
            const username = document.getElementById('reg-username').value.trim();
            const email = document.getElementById('reg-email').value.trim();
            const password = document.getElementById('reg-password').value;
            const confirm = document.getElementById('reg-confirm').value;
            const code = document.getElementById('reg-code').value.trim();
            // Inline validation replacing alerts
            showRegisterError('');
            let hasError = false;
            if (!/^[\p{sc=Han}A-Za-z0-9-]{2,15}$/u.test(username)) { markInvalid(uEl, '用户名需为2-15位，仅中文/英文/数字/横杠-'); hasError = true; }
            if (!/^[A-Za-z0-9._-]{6,18}$/.test(password)) { markInvalid(pEl, '密码需为6-18位，仅包含 a-z A-Z 0-9 和 -_.'); hasError = true; }
            if (password !== confirm) { markInvalid(cEl, '两次输入的密码不一致'); hasError = true; }
            if (!email) { const eEl = document.getElementById('reg-email'); markInvalid(eEl, '请输入邮箱'); hasError = true; }
            if (!code) { const c = document.getElementById('reg-code'); markInvalid(c, '请输入邮箱验证码'); hasError = true; }
            if (hasError) { showRegisterError('请检查标红的字段'); return; }
            register(username, email, password, confirm, code).then(() => {
                // 注册成功或失败后，为避免旧图形验证码残留，刷新一次
                try { loadCaptchaIfEnabled(); } catch(_){}
            });
        });
    }

    // 初始加载时尝试加载验证码（如果默认显示注册页不常见，这里作为补充）
    try { loadCaptchaIfEnabled(); } catch(_){}

    const sendCodeBtn = document.getElementById('btn-send-code');
    if (sendCodeBtn) {
        sendCodeBtn.addEventListener('click', async function() {
            const email = document.getElementById('reg-email').value.trim();
            if (!email) { showRegisterError('请先输入邮箱'); const eEl = document.getElementById('reg-email'); markInvalid(eEl, '请输入邮箱'); return; }
            // If captcha UI is visible, require captcha before sending email code
            const wrap = document.getElementById('captcha-wrap');
            const idEl = document.getElementById('captcha-id');
            const ansEl = document.getElementById('captcha-answer');
            const needCaptcha = wrap && wrap.style.display !== 'none';
            if (needCaptcha) {
                const cid = (idEl?.value || '').trim();
                const cans = (ansEl?.value || '').trim();
                if (!cid || !cans) {
                    notify('请先完成图形验证码', 'warning');
                    return;
                }
            }
            try {
                // UI 状态：禁用并显示倒计时（60s），避免重复点击
                const origText = sendCodeBtn.textContent;
                sendCodeBtn.disabled = true;
                sendCodeBtn.textContent = '发送中...';
                const controller = new AbortController();
                const timerId = setTimeout(() => controller.abort(), 15000); // 15s 超时
                const body = { email };
                if (needCaptcha) {
                    body.captcha_id = (idEl?.value || '').trim();
                    body.captcha_answer = (ansEl?.value || '').trim();
                }
                const res = await fetch(`${API_BASE}/auth/send-email-code`, {
                    method: 'POST', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(body),
                    signal: controller.signal
                });
                clearTimeout(timerId);
                const data = await res.json();
                if (res.ok && (data.code === 0 || !data.code)) {
                    showRegisterError('验证码已发送，请查收邮箱');
                    // 发送成功后刷新一次图形验证码，避免复用
                    try { await refreshCaptcha(); } catch(_){}
                    // 60s 倒计时
                    let left = 60;
                    sendCodeBtn.textContent = `重新发送(${left}s)`;
                    const timer = setInterval(() => {
                        left -= 1;
                        if (left <= 0) {
                            clearInterval(timer);
                            sendCodeBtn.disabled = false;
                            sendCodeBtn.textContent = origText;
                        } else {
                            sendCodeBtn.textContent = `重新发送(${left}s)`;
                        }
                    }, 1000);
                } else {
                    showRegisterError('发送失败: ' + (data.message || '未知错误'));
                    sendCodeBtn.disabled = false;
                    sendCodeBtn.textContent = origText;
                    // 失败也刷新验证码，防止已被 consume 的 id 重复使用
                    try { await refreshCaptcha(); } catch(_){}
                }
            } catch (e) {
                showRegisterError('发送失败: ' + (e.name === 'AbortError' ? '请求超时' : e.message));
                sendCodeBtn.disabled = false;
                sendCodeBtn.textContent = '发送验证码';
                try { await refreshCaptcha(); } catch(_){}
            }
        });
    }

    // 保障“发帖”按钮可点击（即使内联 onclick 失效）
    const createBtnEl = document.getElementById('create-post-btn');
    if (createBtnEl) {
        createBtnEl.addEventListener('click', function(evt) {
            evt.preventDefault();
            showCreatePostPage();
        });
    }
});

window.onload = function() {
    const token = getToken();
    if (token) {
        apiRequest(`${API_BASE}/auth/me`).then(user => {
            currentUser = user?.data || user || null; // 包含 is_admin
        }).catch(error => {
            console.error('自动登录失败:', error);
            clearToken();
            currentUser = null;
        }).finally(() => {
            updateUI();
            // Apply routing after UI initialized
            routeFromLocation();
            window.onpopstate = function() {
                routeFromLocation();
            };
        });
    } else {
        updateUI();
        routeFromLocation();
        window.onpopstate = function() {
            routeFromLocation();
        };
    }
};