const API_BASE = '/api/v1';

function showRegisterError(msg) {
  const box = document.getElementById('register-alert');
  if (!box) return;
  if (!msg) { box.classList.add('d-none'); box.textContent = ''; return; }
  box.textContent = msg; box.classList.remove('d-none');
}
function markInvalid(el) { if (el) el.classList.add('is-invalid'); }
function clearInvalid(el) { if (el) el.classList.remove('is-invalid'); }

async function loadCaptchaIfEnabled() {
  try {
    const res = await fetch(`${API_BASE}/auth/captcha`);
    const data = await res.json();
    const wrap = document.getElementById('captcha-wrap');
    const img = document.getElementById('captcha-image');
    const idEl = document.getElementById('captcha-id');
    const payload = data.data || data;
    if (res.ok && payload && payload.id && payload.image) {
      wrap.style.display = 'block';
      img.src = payload.image; img.style.display = 'inline-block';
      idEl.value = payload.id;
    } else {
      wrap.style.display = 'none';
    }
  } catch (_) {
    const wrap = document.getElementById('captcha-wrap');
    if (wrap) wrap.style.display = 'none';
  }
}

async function refreshCaptcha() {
  try {
    const res = await fetch(`${API_BASE}/auth/captcha`);
    const data = await res.json();
    const payload = data.data || data;
    const img = document.getElementById('captcha-image');
    const idEl = document.getElementById('captcha-id');
    if (payload && payload.image && payload.id) {
      img.src = payload.image; img.style.display = 'inline-block';
      idEl.value = payload.id;
    }
  } catch(_) {}
}

document.addEventListener('DOMContentLoaded', function(){
  loadCaptchaIfEnabled();
  const refreshBtn = document.getElementById('btn-refresh-captcha');
  if (refreshBtn) refreshBtn.addEventListener('click', refreshCaptcha);

  const uEl = document.getElementById('reg-username');
  const pEl = document.getElementById('reg-password');
  const cEl = document.getElementById('reg-confirm');
  const capAns = document.getElementById('captcha-answer');
  const capId = document.getElementById('captcha-id');
  const emailEl = document.getElementById('reg-email');
  const codeEl = document.getElementById('reg-code');
  const emailBad = document.getElementById('email-send-feedback-bad');
  const emailGood = document.getElementById('email-send-feedback-good');

  function clearEmailFeedback() {
    if (emailBad) { emailBad.textContent = ''; emailBad.style.display = ''; }
    if (emailGood) { emailGood.textContent = ''; emailGood.style.display = ''; }
    if (emailEl) clearInvalid(emailEl);
  }

  if (uEl) uEl.addEventListener('blur', function(){
    const v = (uEl.value||'').trim();
    if (!/^[\p{sc=Han}A-Za-z0-9-]{2,15}$/u.test(v)) markInvalid(uEl); else clearInvalid(uEl);
  });
  if (pEl) pEl.addEventListener('blur', function(){
    const v = pEl.value||'';
    if (!/^[A-Za-z0-9._-]{6,18}$/.test(v)) markInvalid(pEl); else clearInvalid(pEl);
  });
  if (cEl) cEl.addEventListener('blur', function(){
    if ((cEl.value||'') !== (pEl?.value||'')) markInvalid(cEl); else clearInvalid(cEl);
  });
  if (capAns) capAns.addEventListener('blur', async function(){
    const wrap = document.getElementById('captcha-wrap');
    if (!wrap || wrap.style.display === 'none') return;
    const id = (capId?.value||'').trim();
    const ans = (capAns.value||'').trim();
    if (!id || !ans) { markInvalid(capAns); return; }
    try {
      const res = await fetch(`${API_BASE}/auth/captcha/verify`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({ captcha_id: id, captcha_answer: ans }) });
      const payload = await res.json();
      if (!res.ok || !(payload.code === 0 || payload.ok)) markInvalid(capAns); else clearInvalid(capAns);
    } catch (_) { markInvalid(capAns); }
  });

  const sendCodeBtn = document.getElementById('btn-send-code');
  if (sendCodeBtn) sendCodeBtn.addEventListener('click', async function(){
    clearEmailFeedback();
    const email = (emailEl.value||'').trim();
    if (!email) {
      markInvalid(emailEl);
      if (emailBad) emailBad.textContent = '请先输入邮箱';
      return;
    }
    const wrap = document.getElementById('captcha-wrap');
    const needCaptcha = wrap && wrap.style.display !== 'none';
    const body = { email };
    if (needCaptcha) {
      const id = (capId?.value||'').trim();
      const ans = (capAns?.value||'').trim();
      if (!id || !ans) { showRegisterError('请先完成图形验证码'); markInvalid(capAns); return; }
      body.captcha_id = id; body.captcha_answer = ans;
    }
    try {
      const origText = sendCodeBtn.textContent; sendCodeBtn.disabled = true; sendCodeBtn.textContent = '发送中...';
      const res = await fetch(`${API_BASE}/auth/send-email-code`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(body) });
      const data = await res.json();
      if (res.ok && (data.code === 0 || !data.code)) {
        if (emailGood) emailGood.textContent = '验证码已发送，请查收邮箱';
        let left = 60; sendCodeBtn.textContent = `重新发送(${left}s)`;
        const timer = setInterval(()=>{ left--; if (left<=0){clearInterval(timer); sendCodeBtn.disabled=false; sendCodeBtn.textContent=origText;} else { sendCodeBtn.textContent = `重新发送(${left}s)`; } }, 1000);
        try { await refreshCaptcha(); } catch(_){}
      } else {
        if (emailBad) emailBad.textContent = '发送失败：' + (data.message || '未知错误');
        sendCodeBtn.disabled = false; sendCodeBtn.textContent = origText; try { await refreshCaptcha(); } catch(_){}
      }
    } catch (e) {
      if (emailBad) emailBad.textContent = '发送失败：' + (e.message || '请求异常');
      sendCodeBtn.disabled = false; sendCodeBtn.textContent = '发送验证码';
      try { await refreshCaptcha(); } catch(_){}
    }
  });

  const form = document.getElementById('register-form');
  if (form) form.addEventListener('submit', async function(e){
    e.preventDefault(); showRegisterError('');
    const username = (uEl.value||'').trim();
    const password = pEl.value||''; const confirm = cEl.value||'';
    const email = (emailEl.value||'').trim(); const code = (codeEl.value||'').trim();
    let hasError = false;
    if (!/^[\p{sc=Han}A-Za-z0-9-]{2,15}$/u.test(username)) { markInvalid(uEl); hasError = true; }
    if (!/^[A-Za-z0-9._-]{6,18}$/.test(password)) { markInvalid(pEl); hasError = true; }
    if (password !== confirm) { markInvalid(cEl); hasError = true; }
    if (!email) { markInvalid(emailEl); hasError = true; }
    if (!code) { markInvalid(codeEl); hasError = true; }
    if (hasError) { showRegisterError('请检查标红的字段'); return; }
    try {
      const payload = { username, email, password, confirm, code };
      // 即使不再强制校验，也附带图形验证码字段（若填写），以便兼容未来策略
      const wrap = document.getElementById('captcha-wrap');
      if (wrap && wrap.style.display !== 'none') {
        const id = (capId?.value||'').trim(); const ans = (capAns?.value||'').trim();
        if (id && ans) { payload.captcha_id = id; payload.captcha_answer = ans; }
      }
      const res = await fetch(`${API_BASE}/auth/register`, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(payload) });
      const data = await res.json();
      if (res.ok && data && (data.code === 0 || data.data?.token)) {
        showRegisterError('');
        const token = data?.data?.token || data?.token;
        if (token) {
          try { localStorage.setItem('token', token); } catch(_){}
        }
        // 自动登录完成后跳转首页
        window.location.href = '/';
      } else {
        showRegisterError(data?.message || '注册失败，请稍后重试');
        try { await refreshCaptcha(); } catch(_){}
      }
    } catch (err) {
      showRegisterError(err.message || '注册失败，请检查网络后重试');
      try { await refreshCaptcha(); } catch(_){}
    }
  });
});
